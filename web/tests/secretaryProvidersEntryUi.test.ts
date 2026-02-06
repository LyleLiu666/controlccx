import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary drawer links to Providers settings for backend/model/token", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");

  assert.match(drawer, />\s*Providers\s*</);
  assert.match(drawer, /emit\('openProvidersSettings'\)/);
  assert.match(appVue, /@openProvidersSettings=\"openProvidersSettings\"/);
});

