import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Auth settings explains how to configure providers per tool with a clear flow", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  assert.ok(modal.includes("录入新的提供方"));
  for (const s of ["Claude Code", "Codex", "秘书", "授权", "模型"]) {
    assert.ok(modal.includes(s), `expected modal to include ${JSON.stringify(s)}`);
  }
  assert.ok(modal.includes("保存并启用"));
  assert.ok(modal.includes("打开提供方"));
  assert.ok(modal.includes("打开工具"));
  assert.ok(modal.includes("emit('openProviders')"));
  assert.ok(modal.includes("emit('openTools')"));
});
