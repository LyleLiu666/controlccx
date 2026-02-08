import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit query composable and api wire all key filters", () => {
  const api = readText("../src/api.ts");
  const composable = readText("../src/composables/useAudit.ts");

  assert.match(api, /export async function fetchAuditEntries\(query: AuditQuery\)/);
  assert.match(api, /qs\.set\("sources", query\.sources\.join\(","\)\)/);
  assert.match(api, /qs\.set\("q", String\(query\.q\)\)/);
  assert.match(api, /qs\.set\("from", String\(query\.from\)\)/);
  assert.match(api, /qs\.set\("to", String\(query\.to\)\)/);
  assert.match(api, /qs\.set\("task_id", String\(query\.task_id\)\)/);
  assert.match(api, /qs\.set\("run_id", String\(query\.run_id\)\)/);
  assert.match(api, /qs\.set\("streams", query\.streams\.join\(","\)\)/);
  assert.match(api, /qs\.set\("cursor", String\(query\.cursor\)\)/);

  assert.match(composable, /function buildQuery\(cursor\?: string\): AuditQuery/);
  assert.match(composable, /sources: querySources\.value\.slice\(\)/);
  assert.match(composable, /q: String\(queryKeyword\.value/);
  assert.match(composable, /task_id: String\(queryTaskID\.value/);
  assert.match(composable, /run_id: String\(queryRunID\.value/);
  assert.match(composable, /streams: queryStreams\.value\.slice\(\)/);
  assert.match(composable, /cursor: String\(cursor \?\? ""\)\.trim\(\)/);
  assert.match(composable, /await fetchAuditEntries\(buildQuery\(""\)\)/);
});
