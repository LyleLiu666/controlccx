import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit detail highlights KV cache and provider receipt summaries", () => {
  const panel = readText("../src/components/AuditPanel.vue");
  assert.match(panel, /auditDetailInsights/);
  assert.match(panel, />KV Cache</);
  assert.match(panel, />Provider Receipt</);
  assert.match(panel, /detail\.meta\?\.kv_cache/);
  assert.match(panel, /detail\.meta\?\.provider_receipt/);
});
