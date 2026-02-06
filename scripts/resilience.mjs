import net from "node:net";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
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

async function postJson(url, body) {
  return await fetchJson(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

function killBestEffort(pid) {
  if (!pid || typeof pid !== "number") return;
  try {
    process.kill(pid);
  } catch {
    // ignore
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

async function spawnUntilHealthy(spawnFn, isHealthyFn, { timeoutMs = 15_000, name = "daemon" } = {}) {
  const deadline = Date.now() + timeoutMs;
  let lastErr = null;
  while (Date.now() < deadline) {
    const child = spawnFn();
    const exitPromise = new Promise((resolve) => child.once("exit", (code, signal) => resolve({ code, signal })));

    const remainingMs = Math.max(0, deadline - Date.now());
    try {
      const v = await Promise.race([
        waitFor(isHealthyFn, { timeoutMs: remainingMs, intervalMs: 200, name }),
        exitPromise.then(({ code, signal }) => {
          throw new Error(`${name} exited before healthy (code=${code} signal=${signal})`);
        }),
      ]);
      child.unref();
      return v;
    } catch (err) {
      lastErr = err;
      // If the child is still around, make sure it doesn't linger across retries.
      killBestEffort(child.pid);
      await Promise.race([exitPromise, sleep(400)]).catch(() => {});
      await sleep(500);
    }
  }
  throw lastErr ?? new Error(`timeout waiting for ${name}`);
}

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

if (!existsSync(binPath) || !existsSync("web/dist/index.html")) {
  console.error("Missing build artifacts; run `pnpm build` first.");
  process.exit(1);
}

if (isWin) {
  console.error("Resilience smoke is currently supported on non-Windows only.");
  process.exit(1);
}

const port = await pickPort();
const runnerPort = await pickPort();
const secretaryPort = await pickPort();
const base = `http://127.0.0.1:${port}`;
const runnerBaseURL = `http://127.0.0.1:${runnerPort}`;
const secretaryBaseURL = `http://127.0.0.1:${secretaryPort}`;
const dataDir = mkdtempSync(path.join(os.tmpdir(), "controlccx-resilience-"));

function cleanupDir() {
  try {
    rmSync(dataDir, { recursive: true, force: true });
  } catch {
    // best-effort
  }
}
process.on("exit", cleanupDir);

function spawnServer() {
  return spawn(
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
    { stdio: "inherit" }
  );
}

function spawnSecretary() {
  return spawn(
    path.resolve(binPath),
    [
      "--mode",
      "secretaryd",
      "--data-dir",
      dataDir,
      "--runnerd-addr",
      `127.0.0.1:${runnerPort}`,
      "--secretaryd-addr",
      `127.0.0.1:${secretaryPort}`,
    ],
    { stdio: "inherit" }
  );
}

let server = null;
let instanceToken = "";

function instanceTokenBestEffort() {
  if (instanceToken) return instanceToken;
  try {
    return readFileSync(path.join(dataDir, "instance.token"), "utf8").trim();
  } catch {
    return "";
  }
}

let cleaned = false;
async function cleanup() {
  if (cleaned) return;
  cleaned = true;

  // Stop server first so daemon exits do not leak zombies under the server parent.
  try {
    await killChildBestEffort(server, { name: "server" });
  } catch {
    // best-effort
  }

  const token = instanceTokenBestEffort();
  const headers = { Accept: "application/json" };
  if (token) headers["X-ControlCCX-Token"] = token;

  try {
    const run = await fetchJson(`${runnerBaseURL}/health`, { headers });
    killBestEffort(run?.pid);
  } catch {
    // ignore
  }

  try {
    const sec = await fetchJson(`${secretaryBaseURL}/health`, { headers });
    killBestEffort(sec?.pid);
  } catch {
    // ignore
  }

  cleanupDir();
}

try {
  server = spawnServer();
  await waitFor(async () => {
    const res = await fetch(`${base}/api/system`);
    return res.ok;
  }, { name: "server /api/system" });

  try {
    instanceToken = readFileSync(path.join(dataDir, "instance.token"), "utf8").trim();
  } catch {
    // ignore
  }

  await postJson(`${base}/api/tools/upsert`, {
    tool: { id: "exec", driver: "exec", command: "sh", args: [] },
  });

async function startTickTask(label, ticks = 12) {
  const script = `echo "${label}: start"\ni=0\nwhile [ $i -lt ${ticks} ]; do echo "${label}: tick $i"; i=$((i+1)); sleep 1; done\necho "${label}: end"\n`;
  const task = await postJson(`${base}/api/tasks`, {
    worker_type: "exec",
    mode: "new",
    prompt: script,
    workdir: dataDir,
  });
  if (!task?.id) throw new Error("task create response missing id");
  return task.id;
}

async function fetchLogs(taskId) {
  const out = await fetchJson(`${base}/api/tasks/${taskId}/logs?after=0&limit=2000`);
  return out?.logs ?? [];
}

function maxTickFromLogs(logs, label) {
  let max = -1;
  for (const l of logs) {
    const msg = String(l?.message ?? "");
    const m = msg.match(new RegExp(`${label}: tick (\\d+)`));
    if (!m) continue;
    const n = Number(m[1]);
    if (Number.isFinite(n) && n > max) max = n;
  }
  return max;
}

async function waitForTaskDone(taskId, timeoutMs = 30_000) {
  return await waitFor(async () => {
    const t = await fetchJson(`${base}/api/tasks/${taskId}`);
    const status = String(t?.status ?? "");
    if (["succeeded", "failed", "blocked", "interrupted", "canceled"].includes(status)) return status;
    return false;
  }, { timeoutMs, intervalMs: 400, name: `task ${taskId} done` });
}

async function controlPlane() {
  return await fetchJson(`${base}/api/control-plane`);
}

// 1) Long-running run survives server restart.
const task1 = await startTickTask("res1", 14);
await waitFor(async () => {
  const logs = await fetchLogs(task1);
  return maxTickFromLogs(logs, "res1") >= 2;
}, { timeoutMs: 12_000, name: "task1 ticks >= 2" });

const beforeKillLogs = await fetchLogs(task1);
const beforeKillTick = maxTickFromLogs(beforeKillLogs, "res1");

const downtimeStart = Date.now();
await killChildBestEffort(server, { name: "server shutdown" });
await sleep(5_000);

server = spawnServer();
await waitFor(async () => (await fetch(`${base}/api/system`)).ok, { name: "server restarted /api/system" });
const downtimeEnd = Date.now();

const afterRestartLogs = await fetchLogs(task1);
const afterRestartTick = maxTickFromLogs(afterRestartLogs, "res1");
if (afterRestartTick <= beforeKillTick) {
  throw new Error(`task1 did not progress during server downtime: before=${beforeKillTick} after=${afterRestartTick}`);
}

let sawDowntimeTick = false;
for (const l of afterRestartLogs) {
  const msg = String(l?.message ?? "");
  if (!msg.includes("res1: tick")) continue;
  const ts = Date.parse(String(l?.time ?? ""));
  if (Number.isFinite(ts) && ts >= downtimeStart && ts <= downtimeEnd) {
    sawDowntimeTick = true;
    break;
  }
}
if (!sawDowntimeTick) {
  throw new Error("task1 logs did not show ticks during the server-down window (unexpected)");
}

const status1 = await waitForTaskDone(task1, 40_000);
if (status1 !== "succeeded") {
  throw new Error(`task1 unexpected status: ${status1}`);
}

// 2) Secretary restart does not affect runner task lifecycle.
const task2 = await startTickTask("res2", 10);
await waitFor(async () => {
  const logs = await fetchLogs(task2);
  return maxTickFromLogs(logs, "res2") >= 1;
}, { timeoutMs: 10_000, name: "task2 ticks >= 1" });

const cp1 = await controlPlane();
if (cp1?.secretaryd?.ok !== true || !Number.isFinite(cp1?.secretaryd?.pid) || cp1.secretaryd.pid <= 0) {
  throw new Error(`expected secretaryd running before restart, got: ${JSON.stringify(cp1?.secretaryd ?? null)}`);
}
const secretaryPid = cp1.secretaryd.pid;
killBestEffort(secretaryPid);

// Bring secretary back.
const token = instanceTokenBestEffort();
await spawnUntilHealthy(
  spawnSecretary,
  async () => {
    const headers = { Accept: "application/json", "X-ControlCCX-Token": token };
    const sec = await fetchJson(`${secretaryBaseURL}/health`, { headers });
    if (sec?.ok !== true || sec?.name !== "secretaryd") return false;
    if (sec?.pid === secretaryPid) return false;
    return true;
  },
  { timeoutMs: 25_000, name: "secretaryd healthy after restart" }
);

const status2 = await waitForTaskDone(task2, 30_000);
if (status2 !== "succeeded") {
  throw new Error(`task2 unexpected status: ${status2}`);
}

// 3) Killing runnerd degrades task plane but keeps secretary explicit.
const cp2 = await controlPlane();
const runnerPid = cp2?.runnerd?.pid;
killBestEffort(runnerPid);

await waitFor(async () => {
  const cp = await controlPlane();
  return cp?.runnerd?.ok === false && cp?.secretaryd?.name;
}, { timeoutMs: 10_000, name: "runnerd degraded" });

const res = await fetch(`${base}/api/tasks`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ worker_type: "exec", mode: "new", prompt: "echo hi", workdir: dataDir }),
});
if (res.status !== 503) {
  const text = await res.text();
  throw new Error(`expected 503 when runner down, got ${res.status}: ${text.slice(0, 200)}`);
}

console.log("Resilience OK");
} finally {
  await cleanup();
}
