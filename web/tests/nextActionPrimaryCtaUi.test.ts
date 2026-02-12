import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("session detail exposes a single next-action primary CTA with secondary actions in disclosure", () => {
  const appVue = readText("../src/App.vue");
  const apiTs = readText("../src/api.ts");

  assert.ok(apiTs.includes("fetchSessionNextAction"));
  assert.ok(apiTs.includes("executeSessionNextAction"));

  assert.ok(appVue.includes("sessionNextAction"));
  assert.ok(appVue.includes("sessionPrimaryAction"));
  assert.ok(appVue.includes("onPrimarySessionAction"));
  assert.ok(appVue.includes("resumePrimaryAction"));
  assert.ok(appVue.includes('class="nextActionBar"'));

  assert.ok(appVue.includes('class="resumeSecondary"'));
  assert.ok(appVue.includes("更多操作"));
  assert.ok(appVue.includes("手动继续（高级）"));
  assert.ok(appVue.includes("抢占当前并继续"));
});
