import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("App removes secretary entrypoints while keeping providers integration", () => {
  const appVue = readText("../src/App.vue");

  assert.ok(!appVue.includes("SecretaryDrawer"));
  assert.ok(!appVue.includes("secOrb"));
  assert.ok(!appVue.includes("toggleSecretary"));
  assert.ok(!appVue.includes("v-model:chatBackend="));

  assert.match(appVue, /<ProvidersPanel/);
  assert.match(appVue, /v-model:secretaryBackend="providerSecretaryBackend"/);
  assert.match(appVue, /v-model:secretarySimpleHTTPBaseURL="providerSecretarySimpleHTTPBaseURL"/);
});
