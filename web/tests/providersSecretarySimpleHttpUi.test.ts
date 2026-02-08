import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal supports Secretary simple-http auth fields", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /Simple HTTP（Anthropic 兼容）/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPBaseURLModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPAuthTokenModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPApiKeyModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPModelModel\"/);
});

test("App wires Secretary simple-http provider fields", () => {
  const appVue = readText("../src/App.vue");
  assert.ok(!appVue.includes("v-model:secretaryBackend"));
  assert.ok(!appVue.includes("v-model:chatBackend"));
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPBaseURL=\"providerSecretarySimpleHTTPBaseURL\"/,
  );
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPAuthToken=\"providerSecretarySimpleHTTPAuthToken\"/,
  );
  assert.match(appVue, /backend:\s*\"simple-http\"/);
  assert.match(appVue, /simple_http:\s*\{/);
});
