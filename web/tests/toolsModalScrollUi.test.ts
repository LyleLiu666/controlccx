import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Tools modal layout keeps body scrollable on small screens", () => {
  const css = readText("../src/App.css");
  assert.match(
    css,
    /:deep\(\.toolsBody\)\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;[^}]*min-height:\s*0;[^}]*overflow:\s*auto;/s,
  );
  assert.match(css, /:deep\(\.toolsSplit\)\s*\{[^}]*flex:\s*1;[^}]*min-height:\s*0;/s);
});
