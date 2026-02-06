import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Secretary exposes a direct settings entry for provider auth", () => {
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const appVue = readText("../src/App.vue");

  assert.match(drawer, /秘书设置/);
  assert.match(drawer, /认证设置/);
  assert.match(drawer, /openAuthSettings/);

  assert.match(appVue, /@openAuthSettings=\"openAuthSettings\"/);
});
