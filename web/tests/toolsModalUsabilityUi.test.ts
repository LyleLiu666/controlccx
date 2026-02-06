import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Tools modal includes practical setup guidance", () => {
  const modal = readText("../src/components/ToolsSettingsModal.vue");
  assert.ok(modal.includes("怎么新增工具"));
  assert.ok(modal.includes("先点“新建”"));
  assert.ok(modal.includes("再点“保存”"));
});

test("new tool flow defaults to exec driver for custom command tools", () => {
  const app = readText("../src/App.vue");
  assert.match(app, /function startNewTool\(\)[\s\S]*toolEditDriver\.value = "exec";/);
});

