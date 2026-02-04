import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("run detail shows token usage meter when usage is available", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");

  assert.ok(appVue.includes("<RunUsageMeter"));
  assert.ok(appVue.includes(':usage="selectedRunUsage"'));
  assert.ok(css.includes(":deep(.usageMeter) {"));
  assert.ok(css.includes(":deep(.usageBar) {"));
  assert.ok(css.includes(":deep(.usageSeg.input) {"));
  assert.ok(css.includes(":deep(.usageSeg.output) {"));
  assert.ok(css.includes(":deep(.usageRow) {"));
  assert.ok(css.includes(':global(:root[data-theme="dark"]) :deep(.usageBar) {'));
});
