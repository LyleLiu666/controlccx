import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers editor save action carries current target tab", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /\(e: "save", target: "claude" \| "codex" \| "secretary"\): void;/);
  assert.match(panel, /@click="onSaveProfile"/);
  assert.match(panel, /function onSaveProfile\(\)[\s\S]*emit\("save", page\.value\);/s);
});

test("App persists provider tool ownership on save and activate", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /async function saveProviderProfile\(target: "claude" \| "codex" \| "secretary"\)/);
  assert.match(appVue, /tool:\s*target,/);
  assert.match(appVue, /async function activateProviderTarget\(target: "claude" \| "codex" \| "secretary"\)/);
});

test("Provider profile type exposes explicit tool ownership", () => {
  const types = readText("../src/types.ts");
  assert.match(types, /tool\?: "claude" \| "codex" \| "secretary";/);
});
