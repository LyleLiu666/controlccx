import assert from "node:assert/strict";
import test from "node:test";

import { formatSkillToken } from "../src/skillsInvoke.ts";

test("formatSkillToken uses / for Claude Code", () => {
  assert.equal(formatSkillToken("ui-ux-pro-max", "claude-code"), "/ui-ux-pro-max");
});

test("formatSkillToken uses $ for Codex", () => {
  assert.equal(formatSkillToken("ui-ux-pro-max", "codex"), "$ui-ux-pro-max");
});

