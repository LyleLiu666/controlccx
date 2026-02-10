import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("resume composer supports queued continue + preempt actions", () => {
  const apiTs = readText("../src/api.ts");
  const appVue = readText("../src/App.vue");
  const typesTs = readText("../src/types.ts");

  assert.ok(apiTs.includes("preemptSessionContinueWithOptions"));
  assert.ok(apiTs.includes("fetchSessionContinueQueue"));
  assert.ok(typesTs.includes("type QueueAck"));
  assert.ok(appVue.includes("onPreemptResumeTask"));
  assert.ok(appVue.includes("抢占当前并继续"));
  assert.ok(appVue.includes("queued"));
  assert.ok(appVue.includes("sessionContinueQueue"));
});
