import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Env import action is visible only for unsaved new profile", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.ok(!panel.includes("providersEnvImportCard"));
  assert.match(panel, /v-if="!editID\.trim\(\)"/);
  assert.match(panel, /providersSubsectionTitle">授权<\/div>[\s\S]*v-if="!editID\.trim\(\)"[\s\S]*从环境变量填充/s);
});
