import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Header menu exposes a reduced-effects toggle", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /function onToggleReducedFxFromMenu\(\)\s*{[\s\S]*toggleFxReduced\(\);/);
  assert.match(appVue, /class=\"headerMoreItem\"[\s\S]*@click=\"onToggleReducedFxFromMenu\"/);
  assert.match(appVue, /减少特效/);
});

test("Reduced effects disables backdrop-filter globally", () => {
  const appCss = readText("../src/App.css");
  assert.match(appCss, /:global\(:root\[data-fx=\"reduced\"\]\)\s*{[\s\S]*--glass-blur:\s*0px;/);
  assert.match(
    appCss,
    /:global\(:root\[data-fx=\"reduced\"\]\)\s*:deep\(\*\)\s*{[\s\S]*backdrop-filter:\s*none\s*!important;/,
  );
});
