import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Deleting provider requires explicit second confirmation", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /async function deleteProviderProfile\([\s\S]*?window\.confirm\([\s\S]*?删除配置后不可恢复/s,
  );
});
