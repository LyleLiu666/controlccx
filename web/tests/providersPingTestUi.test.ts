import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal includes secretary ping test section", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /连通性测试/);
  assert.match(panel, />\s*Ping 测试\s*</);
  assert.match(panel, /emit\('pingtest'\)/);
});

test("App wires provider ping test handler", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /:pingTesting=\"providerPingTesting\"/);
  assert.match(appVue, /:pingResult=\"providerPingResult\"/);
  assert.match(appVue, /@pingtest=\"runProviderPingTest\"/);
  assert.match(appVue, /await pingtestProvider\(\{\s*\n\s*id: providerEditID\.value\.trim\(\),\s*\n\s*backend,/);
});
