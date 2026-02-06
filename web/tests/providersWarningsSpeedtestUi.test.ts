import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers settings modal surfaces env override warnings and speed test actions", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.match(modal, /authStatus/);
  assert.match(modal, /warnings/);
  assert.match(modal, />\s*速度测试\s*</);
  assert.match(modal, /emit\('speedtest'/);
});

test("App wires Providers warnings and speed test handlers", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /:authStatus=\"authStatus\"/);
  assert.match(appVue, /@speedtest=\"runProviderSpeedTest\"/);
});
