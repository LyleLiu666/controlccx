import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Context/Templates page keeps route wiring and removes duplicate overflow entries", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");
  const contextPanel = readText("../src/components/ContextPanel.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");

  // Route + panel wiring.
  assert.match(appVue, /path === \"\/context\"/);
  assert.match(appVue, /<ContextPanel/);

  // Overflow menu should avoid duplicate Skills/Context entries.
  const headerMoreBlock = appVue.match(
    /<div class="headerMorePopup">([\s\S]*?)<\/div>\s*<\/details>/,
  );
  assert.ok(headerMoreBlock, "expected header overflow popup block");
  const headerMorePopup = headerMoreBlock[1];
  assert.doesNotMatch(headerMorePopup, />\s*技能\s*</);
  assert.doesNotMatch(headerMorePopup, />\s*上下文\s*</);

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
