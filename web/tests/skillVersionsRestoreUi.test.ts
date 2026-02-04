import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("SkillVersionsModal offers a restore action to switch versions safely", () => {
  const modal = readText("../src/components/SkillVersionsModal.vue");
  assert.ok(modal.includes("恢复到该版本"));
  assert.ok(modal.includes("~/.agent/skills/"));
  assert.ok(modal.includes("自动生成一份快照"));
});

test("skills API exposes per-skill version restore endpoint", () => {
  const api = readText("../src/api.ts");
  assert.ok(api.includes("/versions/restore"));
});

