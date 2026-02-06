import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Header menu exposes a Providers entrypoint", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /function onOpenProvidersFromMenu\(\)\s*{[\s\S]*openProvidersSettings\(\);/);
  assert.match(appVue, /class=\"headerMoreItem\"[\s\S]*@click=\"onOpenProvidersFromMenu\"/);
  assert.match(appVue, />\s*Providers\s*</);
});

test("Auth settings can open Providers", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  const appVue = readText("../src/App.vue");

  assert.match(modal, />\s*提供方\s*</);
  assert.match(modal, /emit\('openProviders'\)/);
  assert.match(appVue, /@openProviders=\"openProvidersSettings\"/);
});
