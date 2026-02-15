import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit panel can focus on a secretary run_id", () => {
  const panel = readText("../src/components/AuditPanel.vue");

  assert.match(panel, /function resolvedRunID/);
  assert.match(panel, /function focusRun/);
  assert.match(panel, /querySources\.value = \["secretary_event"\]/);
  assert.match(panel, /queryRunID\.value = rid/);
  assert.match(panel, />\s*只看此 run\s*</);
});
