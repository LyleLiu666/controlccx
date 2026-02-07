import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat sync avoids wiping history by fetching from last id", () => {
  const chat = readText("../src/composables/useSecretaryChat.ts");

  // The chat list can exceed the default limit; avoid resetting to the first page.
  assert.doesNotMatch(chat, /chat\.value = await fetchChat\(\);/);
  assert.match(chat, /fetchChat\([^\n]*after/i);
});
