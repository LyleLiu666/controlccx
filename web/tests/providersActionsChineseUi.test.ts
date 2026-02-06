import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal uses Chinese action labels with clear save/activate semantics", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.ok(modal.includes("保存并启用到 Claude Code"));
  assert.ok(modal.includes("保存并启用到 Codex"));
  assert.ok(modal.includes("保存并启用到 秘书"));
  assert.ok(modal.includes("仅保存"));
  assert.ok(modal.includes("（不切换当前工具）"));
});
