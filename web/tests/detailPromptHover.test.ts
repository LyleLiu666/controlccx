import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("detail prompt is emphasized and reveals full text on hover", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");

  assert.ok(appVue.includes("detailPromptWrap"));
  assert.ok(appVue.includes("detailPromptFull"));

  assert.doesNotMatch(css, /\.detailPrompt\s*\{[^}]*font-size:\s*12px;/s);
  assert.match(css, /\.detailPrompt\s*\{[^}]*font-size:\s*13px;/s);
  assert.match(css, /\.detailPrompt\s*\{[^}]*font-weight:\s*700;/s);

  assert.match(css, /\.detailPromptFull\s*\{[^}]*position:\s*absolute;/s);
  assert.match(css, /\.detailPromptFull\s*\{[^}]*max-height:\s*40vh;/s);

  assert.doesNotMatch(css, /\.detailHeader\.compact \.detailTopLeft\s*\{[^}]*overflow:\s*hidden;/s);
  assert.match(css, /\.detailHeader\.compact \.detailTopLeft\s*\{[^}]*overflow:\s*visible;/s);
});

