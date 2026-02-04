import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("SkillVersionsModal surfaces managed source metadata (git/local/import)", () => {
  const modal = readText("../src/components/SkillVersionsModal.vue");
  assert.ok(modal.includes("来源类型"));
  assert.ok(modal.includes("sourceType"));
  assert.ok(modal.includes("sourceRef"));
  assert.ok(modal.includes("sourceRevision"));
  assert.ok(modal.includes("Revision"));
});

