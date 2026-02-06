import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal supports Secretary simple-http auth fields", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.match(modal, /Simple HTTP Auth \(Anthropic\)/);
  assert.match(modal, /v-model=\"secretarySimpleHTTPBaseURLModel\"/);
  assert.match(modal, /v-model=\"secretarySimpleHTTPAuthTokenModel\"/);
  assert.match(modal, /v-model=\"secretarySimpleHTTPApiKeyModel\"/);
  assert.match(modal, /v-model=\"secretarySimpleHTTPModelModel\"/);
});

test("App wires Secretary simple-http provider fields", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPBaseURL=\"providerSecretarySimpleHTTPBaseURL\"/,
  );
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPAuthToken=\"providerSecretarySimpleHTTPAuthToken\"/,
  );
  assert.match(appVue, /simple_http:\s*\{/);
});
