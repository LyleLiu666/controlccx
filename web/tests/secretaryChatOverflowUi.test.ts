import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary chat messages prevent horizontal overflow for long user text", () => {
  const css = readText("../src/App.css");
  assert.match(css, /:deep\(\.msg\)[\s\S]*min-width:\s*0/);
  assert.match(css, /:deep\(\.msgs\)[\s\S]*min-width:\s*0/);
  assert.match(css, /:deep\(\.msg\s+\.content\)[\s\S]*overflow-wrap:\s*anywhere/);
  assert.match(css, /:deep\(\.msg\s+\.content\.chatMarkdown\s+pre\)[\s\S]*max-width:\s*100%/);
});
