import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Auth settings explains how to add providers and map them to tools", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  assert.ok(modal.includes("录入新的提供方"));
  assert.ok(modal.includes("令牌管理"));
  assert.ok(modal.includes("模型管理"));
  assert.ok(modal.includes("保存并启用"));
  assert.ok(modal.includes("打开提供方"));
  assert.ok(modal.includes("打开工具"));
  assert.ok(modal.includes("emit('openProviders')"));
  assert.ok(modal.includes("emit('openTools')"));
});
