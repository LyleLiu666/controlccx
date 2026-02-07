import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page exposes importing env vars into a new profile", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.ok(!panel.includes("新建并导入环境变量"));
  assert.ok(!panel.includes("检测到可导入的环境变量"));
  assert.ok(!panel.includes("新建配置可选操作"));
  assert.ok(!panel.includes("你可以先手动填写，或从环境变量一次性填充。"));
  assert.ok(panel.includes("从环境导入"));
  assert.match(panel, /providersSubsectionHead/);
  assert.match(panel, /providersSubsectionTitle">授权<\/div>[\s\S]*从环境导入/s);
  assert.match(panel, /emit\(["']importEnv["']/);
});

test("App wires provider env import handler", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /@importEnv="importProvidersFromEnv"/);
  assert.match(appVue, /async function importProvidersFromEnv\(/);
  assert.match(appVue, /if \(providerEditID\.value\.trim\(\)\) \{[\s\S]*仅新建配置可导入环境变量/s);
  assert.match(appVue, /await importProviderEnv\(\{ target \}\)/);
});
