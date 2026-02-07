import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat surfaces send errors inside the drawer UI", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");
  const chat = readText("../src/composables/useSecretaryChat.ts");

  assert.match(chat, /chatSendError/);
  assert.match(drawer, /chatSendError/);
  assert.match(drawer, /secChatError/);
  assert.match(appVue, /chatSendError/);
});

