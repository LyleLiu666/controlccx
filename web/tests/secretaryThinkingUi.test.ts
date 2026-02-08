import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary drawer includes fixed-height thinking ticker", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  assert.match(drawer, /secThinkingPanel/);
  assert.match(drawer, /secThinkingViewport/);
  assert.match(drawer, /v-for="\(line, idx\) in thinkingLines"/);
  assert.match(drawer, /--sec-thinking-lines:\s*3/);
  assert.match(drawer, /max-height:\s*calc\(var\(--sec-thinking-lines\)/);
  assert.ok(!drawer.includes("展开"));
});

test("App passes secretary stream state into drawer", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /:thinkingLines="secretaryThinkingLines"/);
  assert.match(appVue, /:streamingReply="secretaryStreamingReply"/);
});
