import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal supports Secretary simple-http auth fields", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /Simple HTTP（Anthropic 兼容）/);
  assert.match(panel, /v-model=\"secretaryBackendModel\"/);
  assert.match(panel, /value=\"openai-chat\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPBaseURLModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPAuthTokenModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPApiKeyModel\"/);
  assert.match(panel, /v-model=\"secretarySimpleHTTPModelModel\"/);
  assert.match(panel, /v-model=\"secretaryOpenAIBaseURLModel\"/);
  assert.match(panel, /v-model=\"secretaryOpenAIApiKeyModel\"/);
  assert.match(panel, /v-model=\"secretaryOpenAIModelModel\"/);
});

test("App wires Secretary provider backend fields", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /v-model:secretaryBackend=\"providerSecretaryBackend\"/);
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPBaseURL=\"\s*providerSecretarySimpleHTTPBaseURL\s*\"/,
  );
  assert.match(
    appVue,
    /v-model:secretarySimpleHTTPAuthToken=\"\s*providerSecretarySimpleHTTPAuthToken\s*\"/,
  );
  assert.match(appVue, /v-model:secretaryOpenAIBaseURL=\"providerSecretaryOpenAIBaseURL\"/);
  assert.match(appVue, /v-model:secretaryOpenAIApiKey=\"providerSecretaryOpenAIApiKey\"/);
  assert.match(appVue, /backend:\s*providerSecretaryBackend\.value/);
  assert.match(appVue, /simple_http:\s*\{/);
  assert.match(appVue, /openai_chat:\s*\{/);
});
