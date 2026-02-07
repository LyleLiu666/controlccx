import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers save shows clear duplicate-name error and blocks save", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /function providerErrorMessage\(e: unknown\)[\s\S]*?name already exists[\s\S]*?保存失败：配置名称不能重名/s,
  );
  assert.match(
    appVue,
    /async function saveProviderProfile\([\s\S]*?catch \(e: any\) \{[\s\S]*?providersError\.value = providerErrorMessage\(e\);/s,
  );
  assert.match(
    appVue,
    /async function activateProviderTarget\([\s\S]*?catch \(e: any\) \{[\s\S]*?providersError\.value = providerErrorMessage\(e\);/s,
  );
});
