import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("token gate is supported and SSE uses fetch headers", () => {
  const apiTs = readText("../src/api.ts");
  const useTasks = readText("../src/composables/useTasks.ts");
  const appVue = readText("../src/App.vue");
  const modalVue = readText("../src/components/InstanceTokenModal.vue");

  assert.ok(apiTs.includes('export const INSTANCE_TOKEN_HEADER = "X-ControlCCX-Token";'));
  assert.ok(apiTs.includes('export const INSTANCE_TOKEN_REQUIRED_ERROR = "instance_token_required";'));
  assert.ok(apiTs.includes("out[INSTANCE_TOKEN_HEADER] = token"));

  assert.ok(!useTasks.includes("EventSource("), "SSE should not use EventSource (headers required)");
  assert.ok(useTasks.includes('fetch("/api/events"'));
  assert.ok(useTasks.includes("text/event-stream"));
  assert.ok(useTasks.includes("buildInstanceTokenHeaders"));

  assert.ok(appVue.includes("InstanceTokenModal"));
  assert.ok(appVue.includes("<InstanceTokenModal"));
  assert.ok(appVue.includes("openInstanceTokenModal("));

  assert.ok(modalVue.includes("X-ControlCCX-Token"));
  assert.ok(modalVue.includes("~/.controlccx/instance.token"));
});

