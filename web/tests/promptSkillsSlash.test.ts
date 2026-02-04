import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("prompt inputs support / to open skills picker", () => {
  const appVue = readText("../src/App.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");

  assert.ok(appVue.includes("onHomePromptKeyDown"));
  assert.ok(appVue.includes('@keydown="onHomePromptKeyDown"'));
  assert.ok(appVue.includes("onResumePromptKeyDown"));
  assert.ok(appVue.includes("<SkillsInsertModal"));

  assert.ok(newRunModal.includes("openSkillsPicker"));
  assert.ok(newRunModal.includes('<span class="mono">/</span>'));
});

