import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Resume safety selects have readable height (no 28px clipping)", () => {
  const css = readText("../src/App.css");

  assert.doesNotMatch(css, /\.resumeSafetyLabel select\s*\{[^}]*height:\s*28px;/s);
  assert.match(css, /\.resumeSafetyLabel select\s*\{[^}]*min-height:\s*40px;/s);
});

