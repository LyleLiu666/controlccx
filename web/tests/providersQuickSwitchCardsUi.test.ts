import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers saved profiles use card layout with quick activate action", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /class=\"providersProfileCard\"/);
  assert.match(panel, /class=\"providersProfileCardActions\"/);
  assert.match(panel, /class=\"providersCardActionBtn\"/);
  assert.match(panel, /当前启用/);
  assert.ok(panel.includes('@click.stop="onSelectActiveProfileFromCard'), "expected activate click handler bound");
  assert.match(panel, />\s*新建配置\s*</);
  assert.ok(panel.includes('@keydown.enter.prevent="onSelectEditProfileFromCard'), "expected keyboard affordance for cards");
});
