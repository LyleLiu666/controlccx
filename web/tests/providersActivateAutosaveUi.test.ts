import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Activating a provider auto-saves new profiles first", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /async function activateProviderTarget\([\s\S]*?\)[\s\S]*upsertProvider\(/,
  );
});
