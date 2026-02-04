import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("dir picker supports creating a folder", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");

  assert.ok(appVue.includes("Select folder"));
  assert.ok(appVue.includes("New folder"));
  assert.ok(appVue.includes("openDirMkdir"));
  assert.ok(appVue.includes("createDirMkdir"));
  assert.ok(appVue.includes("fsMkdir({ path: v.name, base, recursive: false })"));
  assert.ok(css.includes(".pathActions {"));
  assert.ok(css.includes(".mkdirRow {"));
});

