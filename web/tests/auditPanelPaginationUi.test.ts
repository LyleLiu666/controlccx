import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit composable uses bounded cursor pagination instead of infinite append", () => {
  const composable = readText("../src/composables/useAudit.ts");

  assert.match(composable, /const currentCursor = ref\(""\)/);
  assert.match(composable, /const previousCursors = ref<string\[\]>\(\[\]\)/);
  assert.match(composable, /const pageNumber = ref\(1\)/);

  assert.match(composable, /async function loadPrevPage\(\)/);
  assert.match(composable, /async function loadNextPage\(\)/);

  assert.ok(
    !composable.includes("entries.value = [...entries.value, ...list]"),
    "pagination should replace entries per page, not append forever",
  );
});

test("Audit panel exposes previous/next controls and has explicit content padding", () => {
  const panel = readText("../src/components/AuditPanel.vue");

  assert.match(panel, /class="auditPager full"/);
  assert.match(panel, />上一页<\/button>/);
  assert.match(panel, />下一页<\/button>/);
  assert.ok(!panel.includes(">更多</button>"), "legacy load-more control should be removed");

  assert.match(panel, /\.auditPanel\s*\{[\s\S]*padding:\s*16px;/);
});
