import net from "node:net";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, renameSync, rmSync, writeFileSync } from "node:fs";
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

// Providers smoke: import/export/activate/sync/speedtest should work without external network calls.
try {
  const providers0 = await getJSON(`${base}/api/providers`);
  const initialProfiles = providers0?.profiles;
  if (!providers0 || !(initialProfiles == null || Array.isArray(initialProfiles))) {
    throw new Error("unexpected /api/providers payload");
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
  const importedID = String(imp?.profile?.id ?? "").trim();
  if (!importedID) throw new Error("import did not return a profile id");

  const exportedMasked = await getJSON(`${base}/api/providers/export`);
  const maskedText = JSON.stringify(exportedMasked);
  if (maskedText.includes(anthropicToken) || maskedText.includes(openaiKey)) {
    throw new Error("masked export should not include raw secrets");
  }

  const exported = await getJSON(`${base}/api/providers/export?include_secrets=1`);
  const raw = (exported?.profiles ?? []).find((p) => p?.id === importedID);
  if (!raw) throw new Error("export missing imported profile");
  if (String(raw?.targets?.claude?.auth_token ?? "") !== anthropicToken) {
    throw new Error("imported claude auth_token mismatch");
  }
  if (String(raw?.targets?.codex?.api_key ?? "") !== openaiKey) {
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
  child.kill();
  restoreDiskAssets();
  console.error(`Smoke failed: providers checks: ${e?.message ?? String(e)}`);
  process.exit(1);
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
