import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("needs attention sessions can be dismissed (no longer prompt)", () => {
  const appVue = readText("../src/App.vue");
  const drawerVue = readText("../src/components/SecretaryDrawer.vue");

  assert.match(appVue, /controlccx\.attention_dismissed\.v1/);
  assert.ok(appVue.includes("attentionDismissed"));
  assert.ok(appVue.includes("dismissAttentionSession"));
  assert.ok(appVue.includes("@dismissAttention"));

  assert.ok(drawerVue.includes("dismissAttention"));
  assert.match(drawerVue, /取消提醒|不再提示/);
});
