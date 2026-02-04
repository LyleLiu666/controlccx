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
  assert.ok(css.includes(".usageMeter {"));
  assert.ok(css.includes(".usageSeg.input {"));
  assert.ok(css.includes(".usageSeg.output {"));
});
