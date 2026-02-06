import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Auth settings modal can import env to local secrets", () => {
  const modal = readText("../src/components/AuthSettingsModal.vue");
  const appVue = readText("../src/App.vue");

  assert.ok(modal.includes("保存环境变量到本地"));
  assert.ok(modal.includes("emit('importEnv', 'claude')"));
  assert.ok(modal.includes("emit('importEnv', 'codex')"));
  assert.ok(modal.includes('class="modalNotice"'));
  assert.ok(appVue.includes('@importEnv="importAuthEnv"'));
  assert.ok(appVue.includes("importAuthFromEnv("));
});
