import assert from "node:assert/strict";
import test from "node:test";

import { extractSkillTokenNames, formatSkillToken } from "../src/skillsInvoke.ts";

test("formatSkillToken uses / for Claude Code", () => {
  assert.equal(formatSkillToken("ui-ux-pro-max", "claude-code"), "/ui-ux-pro-max");
});

test("formatSkillToken uses $ for Codex", () => {
  assert.equal(formatSkillToken("ui-ux-pro-max", "codex"), "$ui-ux-pro-max");
});

test("extractSkillTokenNames finds referenced skill tokens", () => {
  assert.deepEqual(extractSkillTokenNames("/code-review-excellence\nDo X\n"), ["code-review-excellence"]);
  assert.deepEqual(extractSkillTokenNames("Use $ui-ux-pro-max now\n"), ["ui-ux-pro-max"]);
  assert.deepEqual(extractSkillTokenNames("/Users/alice/project\n"), []);
  assert.deepEqual(extractSkillTokenNames("$HOME/.config\n"), []);
});
