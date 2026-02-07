import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page exposes import/export actions", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, />\s*从 CLI 导入\s*</);
  assert.match(panel, />\s*导出\s*</);
  assert.match(panel, />\s*导出密钥\s*</);
  assert.match(panel, /emit\('importLive'\)/);
  assert.match(panel, /emit\('export',\s*false\)/);
  assert.match(panel, /emit\('export',\s*true\)/);
});

test("App wires Providers import/export actions", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /@importLive=\"importProvidersFromLive\"/);
  assert.match(appVue, /@export=\"exportProvidersToFile\"/);
});
