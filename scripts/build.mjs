import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import path from "node:path";

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

mkdirSync("dist", { recursive: true });

const webResult = spawnSync("pnpm", ["-C", "web", "build"], { stdio: "inherit" });
if (webResult.status !== 0) process.exit(webResult.status ?? 1);

const goArgs = ["build", "-o", binPath, "./cmd/controlccx-server"];
const goResult = spawnSync("go", goArgs, { stdio: "inherit" });
if (goResult.status !== 0) process.exit(goResult.status ?? 1);

if (!existsSync(binPath)) {
  console.error(`Build failed: binary not found at ${binPath}`);
  process.exit(1);
}
console.log(`Built ${binPath}`);

