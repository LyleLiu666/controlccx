import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("SkillVersionsModal supports git pull update and auto snapshot flow", () => {
  const modal = readText("../src/components/SkillVersionsModal.vue");
  assert.ok(modal.includes("拉取更新"));
  assert.ok(modal.includes("sourceType === 'git'"));
  assert.ok(modal.includes("updateFromSource"));
});

test("skills API exposes per-skill update endpoint", () => {
  const api = readText("../src/api.ts");
  assert.ok(api.includes("/versions/update"));
});
