import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page exposes importing env vars into a new profile", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.ok(panel.includes("新建并导入环境变量"));
  assert.match(panel, /emit\(["']importEnv["']/);
  assert.ok(panel.includes("环境变量仅用于辅助填充，不会覆盖已保存配置"));
});

test("App wires provider env import handler", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /@importEnv="importProvidersFromEnv"/);
  assert.match(appVue, /async function importProvidersFromEnv\(/);
  assert.match(appVue, /await importProviderEnv\(\{ target \}\)/);
});
