import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page surfaces a supported-types catalog for quick setup", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  for (const s of ["快速模板", "Claude Code", "Codex", "秘书", "Anthropic（官方）", "OpenAI（官方）"]) {
    assert.ok(panel.includes(s), `expected Providers page to include ${JSON.stringify(s)}`);
  }
});
