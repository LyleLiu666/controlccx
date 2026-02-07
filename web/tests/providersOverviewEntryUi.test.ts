import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page keeps a single overview entry in top actions", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /class="providersHeaderActions"[\s\S]*?>\s*总览\s*</s);
  assert.ok(!panel.includes("快速开始"), "expected quick-start overview block removed");
  assert.doesNotMatch(panel, /providersEditorHeadActions[\s\S]*?>\s*总览\s*</s);
});
