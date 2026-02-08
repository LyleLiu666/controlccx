import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Tools settings modal body is scrollable with deep-scoped styles", () => {
  const css = readText("../src/App.css");
  assert.match(css, /:deep\(\.toolsBody\)\s*\{[^}]*overflow:\s*auto;/s);
});

