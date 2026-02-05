import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Session workspace actions exist and are narrow-safe", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");

  // Workspace actions in the session detail menu.
  assert.match(appVue, /Open run workspace/);
  assert.match(appVue, /Merge back/);
  assert.match(appVue, /Discard workspace/);
  assert.match(appVue, /selectedSessionWorkspace\?\./);

  // Narrow-safe popup sizing and wrapping actions.
  assert.match(css, /\.detailMorePopup[\s\S]*86vw/);
  assert.match(css, /\.detailMoreActions[\s\S]*flex-wrap:\s*wrap/);
});

