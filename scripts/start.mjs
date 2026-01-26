import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

if (!existsSync(binPath) || !existsSync("web/dist")) {
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
  const args = ["--addr", "127.0.0.1:5174", "--static-dir", "web/dist"];
  const child = spawn(path.resolve(binPath), args, { stdio: "inherit" });
  child.on("exit", (code) => process.exit(code ?? 0));
}

