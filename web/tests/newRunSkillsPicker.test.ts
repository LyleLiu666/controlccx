import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("NewRunModal offers a discoverable skills picker (and / shortcut)", () => {
  const modal = readText("../src/components/NewRunModal.vue");

  assert.match(modal, /选择技能/);
  assert.ok(modal.includes('<span class="mono">/</span>'));
});
