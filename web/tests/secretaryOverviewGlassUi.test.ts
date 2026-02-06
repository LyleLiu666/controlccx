import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary overview uses glass panels with clearer empty state hierarchy", () => {
  const drawerVue = readText("../src/components/SecretaryDrawer.vue");
  const css = readText("../src/App.css");

  assert.match(drawerVue, /secSectionAttention/);
  assert.match(drawerVue, /secSectionBriefing/);
  assert.match(drawerVue, /secSectionSubtitle/);
  assert.match(drawerVue, /secEmptyState/);
  assert.match(drawerVue, /secRows/);

  assert.match(css, /\.secSectionAttention/);
  assert.match(css, /\.secSectionBriefing/);
  assert.match(css, /\.secEmptyState/);
  assert.match(css, /backdrop-filter:\s*blur\(/);
});
