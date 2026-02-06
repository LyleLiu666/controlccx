import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat supports simple-http backend and tool error stream hints", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");
  const chat = readText("../src/composables/useSecretaryChat.ts");

  assert.match(drawer, /option value="simple-http"/);
  assert.match(drawer, /chatStreamToolError/);
  assert.match(drawer, /secStreamToolError/);

  assert.match(chat, /"simple-http"/);
  assert.match(chat, /result\.ok === false/);
  assert.match(chat, /chatStreamToolError/);

  assert.match(appVue, /chatStreamToolError/);
  assert.match(appVue, /simple-http/);
});

