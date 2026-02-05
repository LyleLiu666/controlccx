import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("onboarding surfaces Claude Code install guide + Volcengine provider invite", () => {
  const appVue = readText("../src/App.vue");
  const authModal = readText("../src/components/AuthSettingsModal.vue");

  for (const text of [appVue, authModal]) {
    assert.ok(
      text.includes("npm install -g @anthropic-ai/claude-code"),
      "install guide should include Claude Code npm command",
    );
    assert.ok(
      text.includes("claude --version"),
      "install guide should include version verification command",
    );
    assert.ok(text.includes("Node.js 18"), "install guide should mention Node.js 18+");
    assert.ok(
      text.includes("volcengine.com/L/N2h_TKPIsvA"),
      "install guide should include Volcengine invite link",
    );
    assert.ok(text.includes("RTGWR7T3"), "install guide should include invite code");
  }
});

test("missing auth auto-opens Auth Settings", () => {
  const appVue = readText("../src/App.vue");
  assert.match(
    appVue,
    /if \(missingAuthText\.value && !authSettingsOpen\.value && !runningSessionsStartupOpen\.value\) openAuthSettings\(\);/,
  );
});

test("Skills entry is visible in header (not hidden under menu)", () => {
  const appVue = readText("../src/App.vue");
  assert.ok(appVue.includes('class="headerSkillsBtn"'), "expected a visible header Skills button");
  assert.match(appVue, /class="headerSkillsBtn"[\s\S]*?>[\s\S]*?技能/);
});
