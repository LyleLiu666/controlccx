import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("useTasks forwards server events via onServerEvent callback", () => {
  const src = readText("../src/composables/useTasks.ts");
  assert.match(src, /onServerEvent\?: \(evt: ServerEvent\) => void/);
  assert.match(src, /opts\.onServerEvent\?\.\(evt\)/);
});

test("useSecretaryChat handles secretary.thinking and secretary.message server events", () => {
  const src = readText("../src/composables/useSecretaryChat.ts");
  assert.match(src, /function handleServerEvent\(evt: ServerEvent\)/);
  assert.match(src, /evt\.type === "secretary\.thinking"/);
  assert.match(src, /evt\.type !== "secretary\.message"/);
  assert.match(src, /appendThinkingLine\(formatThinkingLine\(thinking\)\)/);
  assert.match(src, /messages\.value = \[/);
  assert.match(src, /const keep = 300/);
});

test("App forwards global SSE events to secretary chat handler", () => {
  const app = readText("../src/App.vue");
  assert.match(app, /onServerEvent: \(evt\) => handleSecretaryServerEvent\(evt\)/);
  assert.match(app, /handleServerEvent: handleSecretaryServerEvent/);
});
