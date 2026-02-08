import net from "node:net";
import { spawn } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
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

async function sleep(ms) {
  await new Promise((r) => setTimeout(r, ms));
}

async function waitFor(fn, { timeoutMs = 10_000, intervalMs = 200, name = "condition" } = {}) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const v = await fn();
      if (v) return v;
    } catch {
      // ignore
    }
    await sleep(intervalMs);
  }
  throw new Error(`timeout waiting for ${name}`);
}

async function fetchJson(url, init) {
  const res = await fetch(url, init);
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`HTTP ${res.status} ${url}: ${text.slice(0, 500)}`);
  }
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(`invalid json from ${url}: ${text.slice(0, 200)}`);
  }
}

async function waitForExit(child, { timeoutMs = 10_000, name = "process" } = {}) {
  if (!child) return;
  if (child.exitCode !== null || child.signalCode !== null) return;
  await Promise.race([
    new Promise((resolve) => child.once("exit", resolve)),
    sleep(timeoutMs).then(() => {
      throw new Error(`timeout waiting for ${name} exit`);
    }),
  ]);
}

async function killChildBestEffort(child, { name = "process" } = {}) {
  if (!child) return;
  try {
    child.kill("SIGTERM");
  } catch {
    // ignore
  }
  try {
    await waitForExit(child, { timeoutMs: 10_000, name });
    return;
  } catch {
    // ignore
  }
  try {
    child.kill("SIGKILL");
  } catch {
    // ignore
  }
  await waitForExit(child, { timeoutMs: 5_000, name: `${name} (SIGKILL)` });
}

function readLinesBestEffort(p) {
  if (!existsSync(p)) return [];
  const txt = readFileSync(p, "utf8");
  return txt.split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
}

function withPrependedPath(env, dir) {
  const next = { ...env };
  const key = Object.prototype.hasOwnProperty.call(env, "PATH") ? "PATH" : "Path";
  const cur = String(env[key] ?? "");
  next[key] = `${dir}${path.delimiter}${cur}`;
  // Keep both keys in sync for Windows weirdness.
  next.PATH = next[key];
  next.Path = next[key];
  return next;
}

function writeBrowserOpenStub(stubDir, openLog) {
  const isWin = process.platform === "win32";
  const isMac = process.platform === "darwin";
  const name = isWin ? "rundll32.cmd" : isMac ? "open" : "xdg-open";
  const p = path.join(stubDir, name);

  if (isWin) {
    writeFileSync(
      p,
      [
        "@echo off",
        "if not \"%CONTROLCCX_OPEN_LOG%\"==\"\" (",
        "  echo %*>>\"%CONTROLCCX_OPEN_LOG%\"",
        ")",
        "exit /b 0",
        "",
      ].join("\r\n"),
      "utf8"
    );
  } else {
    writeFileSync(
      p,
      [
        "#!/bin/sh",
        "if [ -n \"$CONTROLCCX_OPEN_LOG\" ]; then",
        "  printf '%s\\n' \"$*\" >> \"$CONTROLCCX_OPEN_LOG\"",
        "fi",
        "exit 0",
        "",
      ].join("\n"),
      "utf8"
    );
    chmodSync(p, 0o755);
  }

  // Ensure the log file exists and is writable.
  writeFileSync(openLog, "", "utf8");
  return p;
}

function killBestEffort(pid) {
  if (!pid || typeof pid !== "number") return;
  try {
    process.kill(pid);
  } catch {
    // ignore
  }
}

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

if (!existsSync(binPath) || !existsSync("web/dist/index.html")) {
  console.error("Missing build artifacts; run `pnpm build` first.");
  process.exit(1);
}

const port = await pickPort();
const runnerPort = await pickPort();

const base = `http://127.0.0.1:${port}`;
const runnerBaseURL = `http://127.0.0.1:${runnerPort}`;
const expectedURL = `http://127.0.0.1:${port}`;

const tmpDir = mkdtempSync(path.join(os.tmpdir(), "controlccx-startup-open-"));
const dataDir = path.join(tmpDir, "data");
const stubDir = path.join(tmpDir, "bin");
const openLog = path.join(tmpDir, "open.log");
writeFileSync(path.join(tmpDir, ".keep"), "", "utf8");

function cleanupDir() {
  try {
    rmSync(tmpDir, { recursive: true, force: true });
  } catch {
    // best-effort
  }
}
process.on("exit", cleanupDir);

rmSync(dataDir, { recursive: true, force: true });
rmSync(stubDir, { recursive: true, force: true });
mkdirSync(dataDir, { recursive: true });
mkdirSync(stubDir, { recursive: true });

writeBrowserOpenStub(stubDir, openLog);

let instanceToken = "";
let server = null;

async function waitForServer() {
  await waitFor(async () => {
    const res = await fetch(`${base}/api/system`);
    return res.ok;
  }, { timeoutMs: 12_000, name: "server /api/system" });

  // Best-effort read after server is up.
  try {
    instanceToken = readFileSync(path.join(dataDir, "instance.token"), "utf8").trim();
  } catch {
    // ignore
  }
}

async function killDaemonsBestEffort() {
  const headers = instanceToken ? { "X-ControlCCX-Token": instanceToken, Accept: "application/json" } : { Accept: "application/json" };
  try {
    const run = await fetchJson(`${runnerBaseURL}/health`, { headers });
    killBestEffort(run?.pid);
  } catch {
    // ignore
  }
}

function spawnServer(args, { stdio = "inherit" } = {}) {
  const env = withPrependedPath(
    {
      ...process.env,
      CONTROLCCX_OPEN_LOG: openLog,
    },
    stubDir
  );

  return spawn(
    path.resolve(binPath),
    [
      "--data-dir",
      dataDir,
      "--addr",
      `127.0.0.1:${port}`,
      "--runnerd-addr",
      `127.0.0.1:${runnerPort}`,
      ...args,
    ],
    { stdio, env }
  );
}

function assertNoOpenCalls() {
  const lines = readLinesBestEffort(openLog);
  if (lines.length !== 0) {
    throw new Error(`expected no browser open attempts, got ${lines.length}: ${JSON.stringify(lines.slice(0, 5))}`);
  }
}

async function waitForOpenCallCount(min) {
  await waitFor(
    () => {
      const lines = readLinesBestEffort(openLog);
      if (lines.length < min) return false;
      const matched = lines.filter((l) => l.includes(expectedURL));
      if (matched.length < min) return false;
      return true;
    },
    { timeoutMs: 12_000, name: `browser open calls >= ${min}` }
  );
}

try {
  // 1) First launch auto-opens browser to correct URL.
  server = spawnServer([]);
  await waitForServer();
  await waitForOpenCallCount(1);

  // 2) Second launch opens existing instance and exits quickly.
  const second = spawnServer([]);
  await waitForExit(second, { timeoutMs: 2_000, name: "second launch" });
  if (second.exitCode !== 0) {
    throw new Error(`second launch exit code=${second.exitCode} signal=${second.signalCode}`);
  }
  await waitForOpenCallCount(2);

  // 3) --no-open disables auto-open (both first launch and second launch).
  await killChildBestEffort(server, { name: "server shutdown" });
  server = null;
  await sleep(800);

  writeFileSync(openLog, "", "utf8");

  server = spawnServer(["--no-open"]);
  await waitForServer();
  await sleep(900);
  assertNoOpenCalls();

  const secondNoOpen = spawnServer(["--no-open"]);
  await waitForExit(secondNoOpen, { timeoutMs: 2_000, name: "second launch --no-open" });
  if (secondNoOpen.exitCode !== 0) {
    throw new Error(`second launch --no-open exit code=${secondNoOpen.exitCode} signal=${secondNoOpen.signalCode}`);
  }
  await sleep(400);
  assertNoOpenCalls();

  console.log("Startup auto-open OK");
} finally {
  await killChildBestEffort(server, { name: "server cleanup" }).catch(() => {});
  await killDaemonsBestEffort().catch(() => {});
  cleanupDir();
}
