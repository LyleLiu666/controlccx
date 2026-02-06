import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("setup hint uses dark-mode override when theme is dark", () => {
  const css = readText("../src/App.css");
  assert.match(
    css,
    /:global\(:root\[data-theme="dark"\]\)\s*:deep\(\.setupHint\)\s*\{/,
  );
});

