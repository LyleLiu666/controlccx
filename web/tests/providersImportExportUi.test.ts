import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers settings modal exposes import/export actions", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.match(modal, />\s*Import live\s*</);
  assert.match(modal, />\s*Export\s*</);
  assert.match(modal, />\s*Export secrets\s*</);
  assert.match(modal, /emit\('importLive'\)/);
  assert.match(modal, /emit\('export',\s*false\)/);
  assert.match(modal, /emit\('export',\s*true\)/);
});

test("App wires Providers import/export actions", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /@importLive=\"importProvidersFromLive\"/);
  assert.match(appVue, /@export=\"exportProvidersToFile\"/);
});

