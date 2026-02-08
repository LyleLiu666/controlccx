import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit entry is nested under settings instead of header menu", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  const appVue = readText("../src/App.vue");

  assert.match(modal, /\(e: "openAudit"\): void;/);
  assert.match(modal, />\s*审计日志\s*</);

  assert.match(appVue, /@openAudit="openAuditPage"/);
  assert.match(appVue, /function openAuditPage\(\)/);

  assert.ok(!appVue.includes("onOpenAuditFromMenu"));
  const headerMore = appVue.match(/<details[^>]*class="headerMore"[\s\S]*?<\/details>/);
  assert.ok(headerMore, "expected header more block");
  assert.ok(!String(headerMore?.[0] ?? "").includes("审计日志"));
});
