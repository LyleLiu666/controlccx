import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Tools modal is localized to Chinese", () => {
  const modal = readText("../src/components/ToolsSettingsModal.vue");
  for (const s of [
    "工具设置",
    "刷新",
    "工具列表",
    "保存中...",
    "关闭",
    "恢复默认",
    "保存",
  ]) {
    assert.ok(modal.includes(s), `expected Tools modal to include ${JSON.stringify(s)}`);
  }
});
