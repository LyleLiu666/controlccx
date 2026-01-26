import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";

const isWin = process.platform === "win32";
const binName = isWin ? "controlccx.exe" : "controlccx";
const binPath = path.join("dist", binName);

mkdirSync("dist", { recursive: true });

// Keep web/dist non-empty for Go embed; clean build output (except placeholder) for reproducible builds.
const webDistDir = path.join("web", "dist");
const placeholderPath = path.join(webDistDir, "placeholder.txt");
mkdirSync(webDistDir, { recursive: true });
if (!existsSync(placeholderPath)) {
  writeFileSync(
    placeholderPath,
    "This file keeps web/dist non-empty so Go embed works before the first web build.\n"
  );
}
for (const name of readdirSync(webDistDir)) {
  if (name === "placeholder.txt") continue;
  rmSync(path.join(webDistDir, name), { recursive: true, force: true });
}

const webResult = spawnSync("pnpm", ["-C", "web", "build"], { stdio: "inherit" });
if (webResult.status !== 0) process.exit(webResult.status ?? 1);

if (!existsSync(path.join(webDistDir, "index.html"))) {
  console.error(`Build failed: web assets not found at ${path.join(webDistDir, "index.html")}`);
  process.exit(1);
}

const goArgs = ["build", "-o", binPath, "./cmd/controlccx-server"];
const goResult = spawnSync("go", goArgs, { stdio: "inherit" });
if (goResult.status !== 0) process.exit(goResult.status ?? 1);

if (!existsSync(binPath)) {
  console.error(`Build failed: binary not found at ${binPath}`);
  process.exit(1);
}
console.log(`Built ${binPath}`);
