import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal uses Chinese action labels with clear save/activate semantics", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  for (const s of ["令牌管理", "模型管理", "保存并启用", "立即生效"]) {
    assert.ok(panel.includes(s), `expected Providers page to include ${JSON.stringify(s)}`);
  }
  assert.ok(!panel.includes("挂钩"), "expected Providers page to avoid confusing 挂钩 wording");
});
