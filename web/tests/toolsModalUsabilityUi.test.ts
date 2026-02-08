import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Tools modal focuses on CLI configuration (no custom tools)", () => {
  const modal = readText("../src/components/ToolsSettingsModal.vue");
  assert.ok(modal.includes("这里能做什么"));
  assert.ok(modal.includes("只支持配置 Claude Code / Codex"));
  assert.ok(modal.includes("恢复默认"));
});

test("App removes custom tool creation flow", () => {
  const app = readText("../src/App.vue");
  assert.ok(!app.includes("function startNewTool"), "expected startNewTool to be removed");
  assert.ok(!app.includes("@newTool"), "expected ToolsSettingsModal newTool event to be removed");
});
