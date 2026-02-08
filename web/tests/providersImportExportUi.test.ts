import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page exposes import/export actions", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, />\s*导入\s*</);
  assert.match(panel, />\s*导出\s*</);
  assert.ok(!panel.includes("导出密钥"));
  assert.match(panel, /emit\('importFile'/);
  assert.ok(!panel.includes("emit('importLive'"));
  assert.match(panel, /emit\('export',\s*true\)/);
  assert.ok(!panel.includes("emit('export', false)"));
});

test("App wires Providers import/export actions", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /@importFile=\"importProvidersFromFile\"/);
  assert.ok(!appVue.includes("@importLive=\"importProvidersFromLive\""));
  assert.match(appVue, /@export=\"exportProvidersToFile\"/);
});
