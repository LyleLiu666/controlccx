import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Live drawer feed styles are :deep-scoped so they apply inside child components", () => {
  const css = readText("../src/App.css");
  for (const required of [
    ":deep(.secFeed)",
    ":deep(.feedControls)",
    ":deep(.feedBox)",
    ":deep(.feedLine)",
    ":deep(.feedMsg)",
  ]) {
    assert.ok(css.includes(required), `App.css should include ${required}`);
  }
});

