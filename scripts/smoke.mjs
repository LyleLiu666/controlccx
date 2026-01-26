import net from "node:net";
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
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

if (!existsSync(binPath) || !existsSync("web/dist")) {
  console.error("Missing build artifacts; run `pnpm build` first.");
  process.exit(1);
}

const port = await pickPort();
const base = `http://127.0.0.1:${port}`;

const child = spawn(path.resolve(binPath), ["--addr", `127.0.0.1:${port}`, "--static-dir", "web/dist"], {
  stdio: "inherit",
});

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

const sys = await (await fetch(`${base}/api/system`)).json();
if (!sys || !sys.os || !sys.arch) {
  child.kill();
  console.error("Smoke failed: unexpected /api/system response");
  process.exit(1);
}

const html = await (await fetch(`${base}/`)).text();
if (!html.includes("ControlCCX")) {
  child.kill();
  console.error("Smoke failed: UI not served");
  process.exit(1);
}

child.kill();
console.log("Smoke OK");

