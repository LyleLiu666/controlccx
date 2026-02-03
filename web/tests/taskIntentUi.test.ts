import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("manual safety UI relies on safety preset (no separate task intent picker)", () => {
  const appVue = readText("../src/App.vue");
  const newRunModalVue = readText("../src/components/NewRunModal.vue");

  // New run / resume safety UIs should not offer two independent pickers.
  assert.doesNotMatch(appVue, /v-model="resumeTaskIntent"/);
  assert.doesNotMatch(newRunModalVue, /update:taskIntent/);

  // Copy should not claim users must pick intent + preset.
  assert.doesNotMatch(appVue, /手动设置意图\/预设/);
  assert.doesNotMatch(newRunModalVue, /手动设置意图\/预设/);
  assert.match(appVue, /手动设置预设/);
  assert.match(newRunModalVue, /手动设置预设/);
});

