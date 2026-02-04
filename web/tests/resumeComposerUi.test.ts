import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("resume prompt composer is visually prominent and advertises keyboard shortcuts", () => {
  const appVue = readText("../src/App.vue");
  const css = readText("../src/App.css");

  assert.ok(appVue.includes('class="resumeBar"'));
  assert.ok(appVue.includes('class="resumeComposerTop"'));
  assert.ok(appVue.includes('class="resumeComposerHint"'));
  assert.ok(appVue.includes('class="kbd"'));

  assert.ok(
    appVue.includes('@keydown.ctrl.enter="onResumeEnter"') ||
      appVue.includes("@keydown.ctrl.enter=\"onResumeEnter\""),
    "expanded resume textarea should support Ctrl+Enter submit",
  );
  assert.ok(
    appVue.includes('@keydown.meta.enter="onResumeEnter"') ||
      appVue.includes("@keydown.meta.enter=\"onResumeEnter\""),
    "expanded resume textarea should support Meta+Enter submit",
  );

  assert.ok(css.includes(".resumeBar {"));
  assert.ok(css.includes(".resumeComposerTop {"));
  assert.ok(css.includes(".resumeComposerDot {"));
  assert.ok(css.includes(".kbd {"));
  assert.ok(
    css.includes("border: 1px solid rgba(13, 148, 136, 0.22);"),
    "composer container should have a stronger border to stand out",
  );
});
