import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page scopes saved profiles and activation options to current target tab", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /function profileMatchesTarget\(/);
  assert.match(panel, /const pageProfiles = computed<ProviderProfile\[]>\(/);
  assert.match(panel, /v-for="p in pageProfiles"/);
  assert.match(panel, /!pageProfiles\.length/);
  assert.match(panel, /:disabled="saving \|\| !pageProfiles\.length"/);
  assert.match(panel, /const targetProfiles = profilesForTarget\(t\)/);
});

