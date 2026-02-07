import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat streaming is mandatory (no Stream toggle)", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");
  const chat = readText("../src/composables/useSecretaryChat.ts");

  assert.doesNotMatch(drawer, /update:chatStreamEnabled/);
  assert.doesNotMatch(drawer, /chatStreamEnabledModel/);
  assert.doesNotMatch(drawer, /v-model=\"chatStreamEnabledModel\"/);

  assert.doesNotMatch(appVue, /LS_KEY_CHAT_STREAM/);
  assert.doesNotMatch(appVue, /v-model:chatStreamEnabled/);

  assert.doesNotMatch(chat, /chatStreamEnabled/);
});

