import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Context/Templates page is wired with task templates quick-insert (narrow layout safe)", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");
  const contextPanel = readText("../src/components/ContextPanel.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");

  // Route + entrypoint.
  assert.match(appVue, /path === \"\/context\"/);
  assert.match(appVue, /headerMoreItem[\s\S]*?上下文/);
  assert.match(appVue, /<ContextPanel/);

  // Context panel strings (page content).
  assert.match(contextPanel, /Project Context/);
  assert.match(contextPanel, /Prompt Templates/);

  // New Run quick-insert (task templates).
  assert.match(newRunModal, /newRunTemplatesRow/);
  assert.match(newRunModal, /fetchPromptTemplates\(\"task\"\)/);
  assert.match(newRunModal, /应用/);

  // Narrow layout handling should avoid overcrowding in the Context panel.
  assert.match(css, /\.contextPagePanel\s*{/);
  assert.match(css, /@container \(max-width: 520px\)[\s\S]*?\.contextGrid/);
});
