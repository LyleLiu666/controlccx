import net from "node:net";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, renameSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";

async function pickPort() {
  return await new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.on("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      if (!addr || typeof addr === "string") return reject(new Error("unexpected address"));
      const port = addr.port;
      srv.close(() => resolve(port));
    });
  });
}

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

if (!existsSync(binPath) || !existsSync("web/dist/index.html")) {
  console.error("Missing build artifacts; run `pnpm build` first.");
  process.exit(1);
}

// Prove the server serves embedded assets: temporarily hide disk assets.
const diskIndex = path.join("web", "dist", "index.html");
const diskAssets = path.join("web", "dist", "assets");
const bakIndex = path.join("web", "dist", "index.html.bak");
const bakAssets = path.join("web", "dist", "assets.bak");

let moved = false;
function restoreDiskAssets() {
  if (!moved) return;
  try {
    if (existsSync(bakIndex)) renameSync(bakIndex, diskIndex);
    if (existsSync(bakAssets)) renameSync(bakAssets, diskAssets);
  } catch {
    // best-effort
  } finally {
    moved = false;
  }
}
process.on("exit", restoreDiskAssets);

renameSync(diskIndex, bakIndex);
renameSync(diskAssets, bakAssets);
moved = true;

const port = await pickPort();
const runnerPort = await pickPort();
const secretaryPort = await pickPort();
const base = `http://127.0.0.1:${port}`;
const dataDir = mkdtempSync(path.join(os.tmpdir(), "controlccx-smoke-"));
process.on("exit", () => {
  try {
    rmSync(dataDir, { recursive: true, force: true });
  } catch {
    // best-effort
  }
});

const child = spawn(
  path.resolve(binPath),
  [
    "--no-open",
    "--data-dir",
    dataDir,
    "--addr",
    `127.0.0.1:${port}`,
    "--runnerd-addr",
    `127.0.0.1:${runnerPort}`,
    "--secretaryd-addr",
    `127.0.0.1:${secretaryPort}`,
  ],
  {
  stdio: "inherit",
  }
);

const deadline = Date.now() + 10_000;
let ok = false;
while (Date.now() < deadline) {
  try {
    const res = await fetch(`${base}/api/system`);
    if (res.ok) {
      ok = true;
      break;
    }
  } catch {
    // ignore
  }
  await new Promise((r) => setTimeout(r, 200));
}

if (!ok) {
  child.kill();
  console.error("Smoke failed: server did not respond in time");
  process.exit(1);
}

let instanceToken = "";
try {
  instanceToken = readFileSync(path.join(dataDir, "instance.token"), "utf8").trim();
} catch {
  // ignore (cleanup will be best-effort)
}

const sys = await (await fetch(`${base}/api/system`)).json();
if (!sys || !sys.os || !sys.arch) {
  child.kill();
  console.error("Smoke failed: unexpected /api/system response");
  process.exit(1);
}

const html = await (await fetch(`${base}/`)).text();
if (!html.includes("ControlCCX")) {
  child.kill();
  restoreDiskAssets();
  console.error("Smoke failed: UI not served");
  process.exit(1);
}

const jsCandidates = Array.from(html.matchAll(/\/assets\/[^"']+\.js/g)).map((m) => m[0]);
if (!jsCandidates.length) {
  child.kill();
  restoreDiskAssets();
  console.error("Smoke failed: could not find JS asset reference in HTML");
  process.exit(1);
}
const jsPath =
  jsCandidates.find((p) => /\/assets\/index-/.test(p)) ?? jsCandidates[0];
const jsRes = await fetch(`${base}${jsPath}`);
if (!jsRes.ok) {
  child.kill();
  restoreDiskAssets();
  console.error(`Smoke failed: JS asset not served: ${jsPath}`);
  process.exit(1);
}
const jsText = await jsRes.text();
for (const needle of ["Trace", "Download", "Filter logs...", "logs/export"]) {
  if (!jsText.includes(needle)) {
    child.kill();
    restoreDiskAssets();
    console.error(`Smoke failed: UI missing marker string: ${needle}`);
    process.exit(1);
  }
}

child.kill();
restoreDiskAssets();
// Best-effort cleanup: daemons are detached and may outlive the server process.
async function killDaemon(port) {
  try {
    const headers = instanceToken ? { "X-ControlCCX-Token": instanceToken } : {};
    const res = await fetch(`http://127.0.0.1:${port}/health`, { headers });
    if (!res.ok) return;
    const body = await res.json();
    const pid = body?.pid;
    if (!pid || typeof pid !== "number") return;
    process.kill(pid);
  } catch {
    // ignore
  }
}
await killDaemon(runnerPort);
await killDaemon(secretaryPort);
console.log("Smoke OK");
