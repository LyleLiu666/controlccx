import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat shows actionable guidance when provider auth is missing", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");

  assert.match(drawer, /chatAuthHintText/);
  assert.match(drawer, /ANTHROPIC_AUTH_TOKEN/);
  assert.match(drawer, /OPENAI_API_KEY/);
  assert.match(drawer, /claude \/login/);
  assert.match(appVue, /:authStatus=\"authStatus\"/);
});
