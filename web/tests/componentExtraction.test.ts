import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("App.vue extracts common confirmation modals into components", () => {
  const appVue = readText("../src/App.vue");

  for (const required of [
    "<BlockedPromptModal",
    "<RehydratePromptModal",
    "<HighRiskConfirmModal",
  ]) {
    assert.ok(appVue.includes(required), `App.vue should include ${required}`);
  }
});

test("App.css deep-scopes shared modal utility classes for child components", () => {
  const css = readText("../src/App.css");

  for (const required of [":deep(.smallModal)", ":deep(.smallModal .modalBody)", ":deep(.confirmText)"]) {
    assert.ok(css.includes(required), `App.css should include ${required}`);
  }
});

