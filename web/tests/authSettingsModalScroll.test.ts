import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Auth settings modal body is scrollable (deep-scoped overflow)", () => {
  const css = readText("../src/App.css");
  assert.match(
    css,
    /:deep\(\.settingsModal \.modalBody\)\s*\{[^}]*overflow:\s*auto;/s,
  );
});

