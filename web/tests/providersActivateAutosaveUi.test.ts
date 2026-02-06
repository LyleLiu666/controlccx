import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Activating a provider always saves current editor content before activate", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /async function activateProviderTarget\([\s\S]*?const profile: ProviderProfile[\s\S]*?const res = await upsertProvider\(profile\);[\s\S]*?await activateProvider\(\{ target, id \}\);/s,
  );
});
