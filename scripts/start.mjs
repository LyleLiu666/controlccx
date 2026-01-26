import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

const extraArgs = process.argv.slice(2);

if (!existsSync(binPath) || !existsSync("web/dist/index.html")) {
  const buildScript = path.join(path.dirname(fileURLToPath(import.meta.url)), "build.mjs");
  const build = spawn(process.execPath, [buildScript], { stdio: "inherit" });
  build.on("exit", (code) => {
    if (code !== 0) process.exit(code ?? 1);
    run();
  });
} else {
  run();
}

function run() {
  const args = [];

  const addr = process.env.CONTROLCCX_ADDR ?? "127.0.0.1:5174";
  if (!hasFlag(extraArgs, "--addr")) args.push("--addr", addr);
  if (process.env.CONTROLCCX_DATA_DIR && !hasFlag(extraArgs, "--data-dir")) {
    args.push("--data-dir", process.env.CONTROLCCX_DATA_DIR);
  }
  if (process.env.CONTROLCCX_CLAUDE_PATH && !hasFlag(extraArgs, "--claude-path")) {
    args.push("--claude-path", process.env.CONTROLCCX_CLAUDE_PATH);
  }
  if (process.env.CONTROLCCX_CODEX_PATH && !hasFlag(extraArgs, "--codex-path")) {
    args.push("--codex-path", process.env.CONTROLCCX_CODEX_PATH);
  }
  if (process.env.CONTROLCCX_GITBASH_PATH && !hasFlag(extraArgs, "--gitbash-path")) {
    args.push("--gitbash-path", process.env.CONTROLCCX_GITBASH_PATH);
  }

  args.push(...extraArgs);
  const child = spawn(path.resolve(binPath), args, { stdio: "inherit" });
  child.on("exit", (code) => process.exit(code ?? 0));
}

function hasFlag(argv, flag) {
  return argv.includes(flag);
}
