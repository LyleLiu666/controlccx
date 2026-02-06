import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Auth settings modal is localized to Chinese", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  for (const s of [
    "认证设置",
    "工具",
    "提供方",
    "自动化",
    "关闭",
    "保存中...",
  ]) {
    assert.ok(modal.includes(s), `expected modal to include ${JSON.stringify(s)}`);
  }
});
