import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat uses idempotency key to avoid double-send duplicates", () => {
  const api = readText("../src/api.ts");
  const chat = readText("../src/composables/useSecretaryChat.ts");
  const appVue = readText("../src/App.vue");

  assert.match(api, /Idempotency-Key/);
  assert.match(chat, /buildChatIdempotencyKey/);
  assert.match(chat, /idempotency_key/);
  assert.match(appVue, /idempotency_key/);
});

