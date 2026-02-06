import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal is binding-first and surfaces a supported-types catalog", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  for (const s of [
    "挂钩",
    "提供方库",
    "支持的提供方类型",
    "Claude Code",
    "Codex",
    "秘书",
  ]) {
    assert.ok(modal.includes(s), `expected Providers modal to include ${JSON.stringify(s)}`);
  }
});

