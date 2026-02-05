import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("page refresh surfaces a running-sessions startup modal entrypoint", () => {
  const appVue = readText("../src/App.vue");
  assert.ok(appVue.includes("<RunningSessionsStartupModal"));
  assert.ok(appVue.includes("maybeOpenRunningSessionsStartupModal"));
  assert.match(appVue, /await refresh\(\);[\s\S]*maybeOpenRunningSessionsStartupModal\(\);/);
});

test("RunningSessionsStartupModal supports overlay dismiss", () => {
  const modal = readText("../src/components/RunningSessionsStartupModal.vue");
  assert.ok(modal.includes('class="modalOverlay"'));
  assert.ok(modal.includes("@click.self=\"emit('close')\""));
  assert.ok(modal.includes("点击空白处"));
});

