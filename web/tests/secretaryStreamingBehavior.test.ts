import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat send clears input before stream send", () => {
  const composable = readText("../src/composables/useSecretaryChat.ts");
  assert.match(composable, /input\.value = ""[\s\S]*await sendSecretaryMessageStream\(/);
  assert.match(composable, /thinkingLines\.value = \[\]/);
  assert.match(composable, /streamingReply\.value = ""/);
});

test("Secretary API defines stream sender and event parsing", () => {
  const api = readText("../src/api.ts");
  assert.match(api, /\/api\/secretary\/messages\/stream/);
  assert.match(api, /"Accept": "text\/event-stream"/);
  assert.match(api, /case "delta":/);
  assert.match(api, /case "thinking":/);
  assert.match(api, /case "done":/);
});
