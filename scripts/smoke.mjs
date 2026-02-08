import net from "node:net";
import { spawn } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
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

function deleteEnvKey(env, k) {
  try {
    if (env && Object.prototype.hasOwnProperty.call(env, k)) delete env[k];
  } catch {
    // ignore
  }
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

async function getJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`GET ${url}: ${res.status} ${await res.text()}`);
  return await res.json();
}

async function postJSON(url, body) {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });
  if (!res.ok) throw new Error(`POST ${url}: ${res.status} ${await res.text()}`);
  return await res.json();
}

async function waitForOK(url, { timeoutMS = 10_000 } = {}) {
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // ignore
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`timeout waiting for ${url}`);
}

function createStubCLIs(stubRoot) {
  const dir = path.join(stubRoot, "stubs");
  mkdirSync(dir, { recursive: true });

  const claudeScript = path.join(dir, "stub-claude.mjs");
  const codexScript = path.join(dir, "stub-codex.mjs");

  const common = `
import fs from "node:fs";
import path from "node:path";

function captureEnv(tool) {
  const payload = { tool, cwd: process.cwd(), args: process.argv.slice(2), env: {} };
  if (tool === "claude") {
    payload.env.ANTHROPIC_AUTH_TOKEN = process.env.ANTHROPIC_AUTH_TOKEN ?? "";
    payload.env.ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY ?? "";
    payload.env.ANTHROPIC_BASE_URL = process.env.ANTHROPIC_BASE_URL ?? "";
    payload.env.ANTHROPIC_MODEL = process.env.ANTHROPIC_MODEL ?? "";
    payload.env.ANTHROPIC_SMALL_FAST_MODEL = process.env.ANTHROPIC_SMALL_FAST_MODEL ?? "";
  } else if (tool === "codex") {
    payload.env.OPENAI_API_KEY = process.env.OPENAI_API_KEY ?? "";
  }
  fs.writeFileSync(path.join(process.cwd(), ".ccx-capture.json"), JSON.stringify(payload, null, 2) + "\\n", "utf8");
}

async function maybeSleepFromStdin() {
  let stdin = "";
  try {
    stdin = fs.readFileSync(0, "utf8");
  } catch {
    stdin = "";
  }
  const m = /CCX_SMOKE_SLEEP_MS=(\\d+)/.exec(stdin);
  if (!m) return;
  const ms = Math.max(0, Math.min(Number(m[1]), 5_000));
  if (!Number.isFinite(ms) || ms <= 0) return;
  await new Promise((r) => setTimeout(r, ms));
}
`.trim();

  writeFileSync(
    claudeScript,
    `
${common}
captureEnv("claude");
await maybeSleepFromStdin();
process.stdout.write('{"type":"system","subtype":"init","session_id":"sess-smoke-claude"}\\n');
process.stdout.write('{"type":"assistant","session_id":"sess-smoke-claude","result":"ok"}\\n');
`.trimStart(),
    "utf8",
  );

  writeFileSync(
    codexScript,
    `
${common}
captureEnv("codex");
await maybeSleepFromStdin();
process.stdout.write('{"type":"noop","thread_id":"thr-smoke-codex"}\\n');
process.stdout.write('{"type":"item.completed","thread_id":"thr-smoke-codex","item":{"type":"agent_message","text":"ok"}}\\n');
`.trimStart(),
    "utf8",
  );

  const node = process.execPath;
  if (isWin) {
    const claudeCmd = path.join(dir, "claude.cmd");
    const codexCmd = path.join(dir, "codex.cmd");
    writeFileSync(claudeCmd, `@echo off\r\n\"${node}\" \"${claudeScript}\" %*\r\n`, "utf8");
    writeFileSync(codexCmd, `@echo off\r\n\"${node}\" \"${codexScript}\" %*\r\n`, "utf8");
    return { claudePath: claudeCmd, codexPath: codexCmd };
  }

  const claudeSh = path.join(dir, "claude");
  const codexSh = path.join(dir, "codex");
  writeFileSync(claudeSh, `#!/bin/sh\nexec \"${node}\" \"${claudeScript}\" \"$@\"\n`, "utf8");
  writeFileSync(codexSh, `#!/bin/sh\nexec \"${node}\" \"${codexScript}\" \"$@\"\n`, "utf8");
  chmodSync(claudeSh, 0o755);
  chmodSync(codexSh, 0o755);
  return { claudePath: claudeSh, codexPath: codexSh };
}

async function spawnServer({ env, claudePath = "", codexPath = "" }) {
  const port = await pickPort();
  const runnerPort = await pickPort();
  const base = `http://127.0.0.1:${port}`;
  const dataDir = mkdtempSync(path.join(os.tmpdir(), "controlccx-smoke-"));

  process.on("exit", () => {
    try {
      rmSync(dataDir, { recursive: true, force: true });
    } catch {
      // best-effort
    }
  });

  const args = [
    "--no-open",
    "--data-dir",
    dataDir,
    "--addr",
    `127.0.0.1:${port}`,
    "--runnerd-addr",
    `127.0.0.1:${runnerPort}`,
  ];
  if (claudePath) args.push("--claude-path", claudePath);
  if (codexPath) args.push("--codex-path", codexPath);

  const child = spawn(path.resolve(binPath), args, { stdio: "inherit", env });

  await waitForOK(`${base}/api/system`, { timeoutMS: 10_000 });

  let instanceToken = "";
  try {
    instanceToken = readFileSync(path.join(dataDir, "instance.token"), "utf8").trim();
  } catch {
    instanceToken = "";
  }

  return { child, base, dataDir, port, runnerPort, instanceToken };
}

async function killDaemon(port, instanceToken) {
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

async function shutdownServer({ child, runnerPort, instanceToken }) {
  try {
    child.kill();
  } catch {
    // ignore
  }
  // Best-effort cleanup: daemons are detached and may outlive the server process.
  await killDaemon(runnerPort, instanceToken);
}

async function waitForTaskFinish(base, id, { timeoutMS = 12_000 } = {}) {
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    const task = await getJSON(`${base}/api/tasks/${id}`);
    const st = String(task?.status ?? "").trim();
    if (["succeeded", "failed", "canceled", "interrupted", "blocked"].includes(st)) return task;
    await new Promise((r) => setTimeout(r, 120));
  }
  throw new Error(`timeout waiting for task finish: ${id}`);
}

async function assertUIIsServed(base) {
  const sys = await getJSON(`${base}/api/system`);
  if (!sys || !sys.os || !sys.arch) throw new Error("unexpected /api/system response");

  const html = await (await fetch(`${base}/`)).text();
  if (!html.includes("ControlCCX")) throw new Error("UI not served");

  const jsCandidates = Array.from(html.matchAll(/\/assets\/[^"']+\.js/g)).map((m) => m[0]);
  if (!jsCandidates.length) throw new Error("could not find JS asset reference in HTML");
  const jsPath = jsCandidates.find((p) => /\/assets\/index-/.test(p)) ?? jsCandidates[0];
  const jsRes = await fetch(`${base}${jsPath}`);
  if (!jsRes.ok) throw new Error(`JS asset not served: ${jsPath}`);
  const jsText = await jsRes.text();
  for (const needle of ["Trace", "Download", "Filter logs...", "logs/export"]) {
    if (!jsText.includes(needle)) throw new Error(`UI missing marker string: ${needle}`);
  }
}

// Providers smoke: import/export/activate/sync/speedtest should work without external network calls.
let phaseA;
try {
  const envA = { ...process.env };
  // Ensure env override warnings are exercised deterministically.
  envA.OPENAI_API_KEY = "smoke-env-openai-key";
  envA.ANTHROPIC_AUTH_TOKEN = "smoke-env-anthropic-token";

  phaseA = await spawnServer({ env: envA });
  await assertUIIsServed(phaseA.base);

  const base = phaseA.base;
  const dataDir = phaseA.dataDir;

  const providers0 = await getJSON(`${base}/api/providers`);
  const initialProfiles = providers0?.profiles;
  if (!providers0 || !(initialProfiles == null || Array.isArray(initialProfiles))) {
    throw new Error("unexpected /api/providers payload");
  }

  const authStatus = await getJSON(`${base}/api/auth/status`);
  const warnings = Array.isArray(authStatus?.warnings) ? authStatus.warnings : [];
  const warningsText = warnings.join("\n");
  if (!warningsText.includes("OPENAI_API_KEY") || !warningsText.includes("ANTHROPIC_AUTH_TOKEN")) {
    throw new Error("expected env override warnings in /api/auth/status");
  }
  if (String(authStatus?.codex?.api_key?.effective ?? "") !== "env") {
    throw new Error("expected codex api key effective source to be env");
  }
  if (String(authStatus?.claude?.auth_token?.effective ?? "") !== "env") {
    throw new Error("expected claude auth token effective source to be env");
  }

  const liveRoot = path.join(dataDir, "live");
  const claudeHome = path.join(liveRoot, "claude");
  const codexHome = path.join(liveRoot, "codex");
  mkdirSync(claudeHome, { recursive: true });
  mkdirSync(codexHome, { recursive: true });

  const anthropicToken = "smoke-anthropic-token";
  const openaiKey = "smoke-openai-key";

  writeFileSync(
    path.join(claudeHome, "settings.json"),
    JSON.stringify(
      {
        env: {
          ANTHROPIC_BASE_URL: base,
          ANTHROPIC_AUTH_TOKEN: anthropicToken,
          ANTHROPIC_MODEL: "claude-3-7-sonnet",
        },
      },
      null,
      2,
    ) + "\n",
    "utf8",
  );
  writeFileSync(
    path.join(codexHome, "auth.json"),
    JSON.stringify({ OPENAI_API_KEY: openaiKey }, null, 2) + "\n",
    "utf8",
  );
  writeFileSync(
    path.join(codexHome, "config.toml"),
    'model = "gpt-5.2"\nmodel_reasoning_effort = "high"\n',
    "utf8",
  );

  const imp = await postJSON(`${base}/api/providers/import/live`, {
    name: "Imported",
    claude_home_dir: claudeHome,
    codex_home_dir: codexHome,
  });
  const importedClaudeID = String(imp?.profile?.id ?? "").trim();
  if (!importedClaudeID) throw new Error("import did not return a profile id");

  const exportedMasked = await getJSON(`${base}/api/providers/export`);
  const maskedText = JSON.stringify(exportedMasked);
  if (maskedText.includes(anthropicToken) || maskedText.includes(openaiKey)) {
    throw new Error("masked export should not include raw secrets");
  }

  const exported = await getJSON(`${base}/api/providers/export?include_secrets=1`);
  const profiles = Array.isArray(exported?.profiles) ? exported.profiles : [];
  const rawClaude = profiles.find((p) => p?.id === importedClaudeID);
  if (!rawClaude) throw new Error("export missing imported claude profile");
  if (String(rawClaude?.targets?.claude?.auth_token ?? "") !== anthropicToken) {
    throw new Error("imported claude auth_token mismatch");
  }
  const rawCodex = profiles.find(
    (p) => String(p?.tool ?? "").trim() === "codex" && String(p?.name ?? "").trim() === "Imported Codex",
  );
  if (!rawCodex) throw new Error("export missing imported codex profile");
  if (String(rawCodex?.targets?.codex?.api_key ?? "") !== openaiKey) {
    throw new Error("imported codex api_key mismatch");
  }

  const syncUpsert = await postJSON(`${base}/api/providers/upsert`, {
    profile: {
      name: "SyncTest",
      targets: {
        claude: {
          base_url: base,
          auth_token: "smoke-anthropic-token-next",
          model: "claude-3-7-sonnet",
        },
        codex: {
          base_url: base,
          api_key: "smoke-openai-key-next",
          model: "gpt-5.2",
          reasoning_effort: "medium",
        },
      },
      sync_live: { claude: true, codex: true, secretary: false },
    },
  });
  const syncID = String(syncUpsert?.profile?.id ?? "").trim();
  if (!syncID) throw new Error("sync upsert did not return a profile id");

  await postJSON(`${base}/api/providers/activate`, {
    target: "claude",
    id: syncID,
    claude_home_dir: claudeHome,
  });
  await postJSON(`${base}/api/providers/activate`, {
    target: "codex",
    id: syncID,
    codex_home_dir: codexHome,
  });

  const claudeBackupsDir = path.join(dataDir, "backups", "live", "claude");
  const codexAuthBackupsDir = path.join(dataDir, "backups", "live", "codex", "auth");
  const codexConfigBackupsDir = path.join(dataDir, "backups", "live", "codex", "config");
  if (!existsSync(claudeBackupsDir) || readdirSync(claudeBackupsDir).length === 0) {
    throw new Error("expected claude live backup files");
  }
  if (!existsSync(codexAuthBackupsDir) || readdirSync(codexAuthBackupsDir).length === 0) {
    throw new Error("expected codex auth backup files");
  }
  if (!existsSync(codexConfigBackupsDir) || readdirSync(codexConfigBackupsDir).length === 0) {
    throw new Error("expected codex config backup files");
  }

  const claudeSettings = JSON.parse(readFileSync(path.join(claudeHome, "settings.json"), "utf8"));
  if (String(claudeSettings?.env?.ANTHROPIC_BASE_URL ?? "") !== base) {
    throw new Error("claude live sync base url mismatch");
  }
  if (String(claudeSettings?.env?.ANTHROPIC_MODEL ?? "") !== "claude-3-7-sonnet") {
    throw new Error("claude live sync model mismatch");
  }
  const codexAuth = JSON.parse(readFileSync(path.join(codexHome, "auth.json"), "utf8"));
  if (String(codexAuth?.OPENAI_API_KEY ?? "") !== "smoke-openai-key-next") {
    throw new Error("codex live sync api key mismatch");
  }
  const codexConfig = readFileSync(path.join(codexHome, "config.toml"), "utf8");
  for (const needle of ['model = "gpt-5.2"', 'model_reasoning_effort = "medium"']) {
    if (!codexConfig.includes(needle)) throw new Error(`codex live sync missing: ${needle}`);
  }

  const stClaude = await postJSON(`${base}/api/providers/speedtest`, { target: "claude", id: syncID });
  if (!stClaude?.result?.ok) throw new Error("claude speed test should succeed");
  const stCodex = await postJSON(`${base}/api/providers/speedtest`, { target: "codex", id: syncID });
  if (!stCodex?.result?.ok) throw new Error("codex speed test should succeed");
} catch (e) {
  if (phaseA) await shutdownServer(phaseA);
  restoreDiskAssets();
  console.error(`Smoke failed: providers checks: ${e?.message ?? String(e)}`);
  process.exit(1);
}

if (phaseA) await shutdownServer(phaseA);

// Provider switching should affect newly-started runs when env does not override stored secrets.
let phaseB;
try {
  const envB = { ...process.env };
  for (const k of [
    "OPENAI_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_BASE_URL",
    "ANTHROPIC_MODEL",
    "ANTHROPIC_SMALL_FAST_MODEL",
  ]) {
    deleteEnvKey(envB, k);
  }

  const stubPaths = createStubCLIs(mkdtempSync(path.join(os.tmpdir(), "controlccx-smoke-stubs-")));
  phaseB = await spawnServer({ env: envB, claudePath: stubPaths.claudePath, codexPath: stubPaths.codexPath });

  const upsertA = await postJSON(`${phaseB.base}/api/providers/upsert`, {
    profile: {
      name: "RunProviderA",
      targets: {
        claude: { auth_token: "smoke-claude-token-A", model: "claude-3-7-sonnet" },
        codex: { api_key: "smoke-openai-key-A", model: "gpt-5.2", reasoning_effort: "medium" },
      },
      sync_live: { claude: false, codex: false, secretary: false },
    },
  });
  const idA = String(upsertA?.profile?.id ?? "").trim();
  if (!idA) throw new Error("profile A upsert missing id");

  const upsertB = await postJSON(`${phaseB.base}/api/providers/upsert`, {
    profile: {
      name: "RunProviderB",
      targets: {
        claude: { auth_token: "smoke-claude-token-B", model: "claude-3-5-haiku" },
        codex: { api_key: "smoke-openai-key-B", model: "gpt-5.2", reasoning_effort: "high" },
      },
      sync_live: { claude: false, codex: false, secretary: false },
    },
  });
  const idB = String(upsertB?.profile?.id ?? "").trim();
  if (!idB) throw new Error("profile B upsert missing id");

  async function runTaskAndCapture({ workerType, workDir, expectedKey, expectedTool }) {
    mkdirSync(workDir, { recursive: true });
    const task = await postJSON(`${phaseB.base}/api/tasks`, {
      worker_type: workerType,
      mode: "new",
      prompt: "CCX_SMOKE: verify provider switching",
      workdir: workDir,
    });
    const taskID = String(task?.id ?? "").trim();
    if (!taskID) throw new Error("create task missing id");
    const finished = await waitForTaskFinish(phaseB.base, taskID);
    if (String(finished?.status ?? "") !== "succeeded") {
      throw new Error(`task ${taskID} did not succeed: status=${finished?.status ?? ""}`);
    }
    const trace = await getJSON(`${phaseB.base}/api/tasks/${taskID}/trace`);
    const invDir = String(trace?.invocation?.dir ?? "").trim();
    if (!invDir) throw new Error(`missing invocation.dir for task ${taskID}`);
    const capPath = path.join(invDir, ".ccx-capture.json");
    if (!existsSync(capPath)) throw new Error(`missing capture file: ${capPath}`);
    const cap = JSON.parse(readFileSync(capPath, "utf8"));
    if (String(cap?.tool ?? "") !== expectedTool) throw new Error(`capture tool mismatch: ${cap?.tool ?? ""}`);
    if (expectedTool === "codex") {
      const got = String(cap?.env?.OPENAI_API_KEY ?? "");
      if (got !== expectedKey) throw new Error(`codex OPENAI_API_KEY mismatch: got=${got} want=${expectedKey}`);
    } else if (expectedTool === "claude") {
      const got = String(cap?.env?.ANTHROPIC_AUTH_TOKEN ?? "");
      if (got !== expectedKey) throw new Error(`claude ANTHROPIC_AUTH_TOKEN mismatch: got=${got} want=${expectedKey}`);
    }
  }

  await postJSON(`${phaseB.base}/api/providers/activate`, { target: "codex", id: idA });
  await runTaskAndCapture({
    workerType: "codex",
    workDir: path.join(phaseB.dataDir, "run-a-codex"),
    expectedKey: "smoke-openai-key-A",
    expectedTool: "codex",
  });

  if (!isWin) {
    await postJSON(`${phaseB.base}/api/providers/activate`, { target: "claude", id: idA });
    await runTaskAndCapture({
      workerType: "claude-code",
      workDir: path.join(phaseB.dataDir, "run-a-claude"),
      expectedKey: "smoke-claude-token-A",
      expectedTool: "claude",
    });
  }

  await postJSON(`${phaseB.base}/api/providers/activate`, { target: "codex", id: idB });
  await runTaskAndCapture({
    workerType: "codex",
    workDir: path.join(phaseB.dataDir, "run-b-codex"),
    expectedKey: "smoke-openai-key-B",
    expectedTool: "codex",
  });

  if (!isWin) {
    await postJSON(`${phaseB.base}/api/providers/activate`, { target: "claude", id: idB });
    await runTaskAndCapture({
      workerType: "claude-code",
      workDir: path.join(phaseB.dataDir, "run-b-claude"),
      expectedKey: "smoke-claude-token-B",
      expectedTool: "claude",
    });
  }
} catch (e) {
  if (phaseB) await shutdownServer(phaseB);
  restoreDiskAssets();
  console.error(`Smoke failed: provider switch run checks: ${e?.message ?? String(e)}`);
  process.exit(1);
}

if (phaseB) await shutdownServer(phaseB);
restoreDiskAssets();
console.log("Smoke OK");
