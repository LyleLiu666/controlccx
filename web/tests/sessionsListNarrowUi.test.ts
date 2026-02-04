import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Sessions list is compact (no session id, concise last-run label, narrow layout rules)", () => {
  const appVueUrl = new URL("../src/App.vue", import.meta.url);
  const cssUrl = new URL("../src/App.css", import.meta.url);
  const appVue = readFileSync(appVueUrl, "utf8");
  const css = readFileSync(cssUrl, "utf8");

  // Session id short code is noisy in the list; keep it out of the row template.
  const sessionIdTitleNeedle = ':title="s.session_id || s.latest.id"';
  assert.equal(
    appVue.includes(sessionIdTitleNeedle),
    false,
    `App.vue should not include ${sessionIdTitleNeedle} (read from ${appVueUrl.pathname})`,
  );

  // The time pill should not render the redundant "最后" prefix.
  assert.doesNotMatch(appVue, />最后\\s*<span/);
  assert.ok(appVue.includes("运行时间："));

  // Narrow-width handling should avoid overcrowding in the Sessions panel.
  assert.match(
    css,
    /@container \(max-width: 520px\)[\s\S]*\.sessionsPanel \.rowTop/,
  );
  assert.match(
    css,
    /@container \(max-width: 520px\)[\s\S]*\.sessionsPanel \.rowTopRight/,
  );
});
