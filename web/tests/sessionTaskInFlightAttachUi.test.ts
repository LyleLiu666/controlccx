import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("409 session_task_in_flight auto-attaches to existing run", () => {
  const apiTs = readText("../src/api.ts");
  const appVue = readText("../src/App.vue");

  assert.ok(apiTs.includes("sessionTaskInFlightFromError"));
  assert.ok(appVue.includes("maybeAttachSessionInFlightTask"));
  assert.ok(appVue.includes("sessionTaskInFlightFromError"));
  assert.ok(appVue.includes("selectedTaskId.value = inFlight.taskId"));
  assert.ok(appVue.includes('outputTab.value = "logs"'));
});

