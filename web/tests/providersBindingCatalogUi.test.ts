import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page is overview-first with direct tool entrypoints", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  for (const s of ["总览", "我想配置", "Claude Code", "Codex", "秘书", "已保存配置（点击切换编辑）"]) {
    assert.ok(panel.includes(s), `expected Providers page to include ${JSON.stringify(s)}`);
  }
  assert.ok(!panel.includes("推荐模板"), "expected template section to be removed");
  assert.ok(!panel.includes("正在编辑的配置"), "expected confusing editing dropdown label to be removed");
});
