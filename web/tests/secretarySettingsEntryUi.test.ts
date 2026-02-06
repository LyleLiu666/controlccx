import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary exposes discoverable Providers + auth entrypoints", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");

  assert.match(drawer, /secDrawerHeader[\s\S]*secHeaderAction[\s\S]*openProvidersSettings/);
  assert.match(drawer, /认证设置/);
  assert.match(drawer, /openAuthSettings/);

  assert.match(appVue, /@openProvidersSettings=\"openProvidersSettings\"/);
  assert.match(appVue, /@openAuthSettings=\"openAuthSettings\"/);
});
