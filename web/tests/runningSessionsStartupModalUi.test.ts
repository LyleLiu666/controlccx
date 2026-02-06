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

test("RunningSessionsStartupModal is narrow-screen safe (CSS + semantics)", () => {
  const modal = readText("../src/components/RunningSessionsStartupModal.vue");
  assert.match(modal, /class=\"modal\s+smallModal\b/);
  assert.ok(modal.includes('role="dialog"'));
  assert.ok(modal.includes('aria-modal="true"'));

  const css = readText("../src/App.css");
  assert.match(css, /:deep\(\.modalOverlay\)[\s\S]*padding:\s*24px;/);
  assert.match(css, /:deep\(\.smallModal\)[\s\S]*width:\s*min\(520px,\s*95vw\);/);
  assert.match(css, /:deep\(\.smallModal\)[\s\S]*max-height:\s*90vh;/);
  assert.match(css, /:deep\(\.smallModal\s+\.modalBody\)[\s\S]*overflow:\s*auto;/);

  assert.match(css, /:deep\(\.runningSessionsRowName\)[\s\S]*min-width:\s*0;/);
  assert.match(css, /:deep\(\.runningSessionsRowName\)[\s\S]*overflow:\s*hidden;/);
  assert.match(css, /:deep\(\.runningSessionsRowName\)[\s\S]*text-overflow:\s*ellipsis;/);
  assert.match(css, /:deep\(\.runningSessionsRowName\)[\s\S]*white-space:\s*nowrap;/);

  assert.match(css, /:deep\(\.runningSessionsRowWorkdir\)[\s\S]*min-width:\s*0;/);
  assert.match(css, /:deep\(\.runningSessionsRowWorkdir\)[\s\S]*overflow:\s*hidden;/);
  assert.match(css, /:deep\(\.runningSessionsRowWorkdir\)[\s\S]*text-overflow:\s*ellipsis;/);
});
