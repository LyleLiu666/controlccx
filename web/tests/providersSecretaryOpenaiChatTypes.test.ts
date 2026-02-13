import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("ProviderSecretaryTarget supports openai-chat", () => {
  const types = readText("../src/types.ts");
  assert.match(types, /export type ProviderSecretaryBackend = \"simple-http\" \\| \"openai-chat\";/);
  assert.match(types, /openai_chat\?: \{/);
});
