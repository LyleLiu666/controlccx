import assert from "node:assert/strict";
import test from "node:test";

import { buildRunSafetyPayload } from "../src/runSafety.ts";

test("claude-code safe preset sets acceptEdits permission mode", () => {
  const payload = buildRunSafetyPayload("claude-code", "code", "search-browse");
  assert.equal(payload.claude_permission_mode, "acceptEdits");
  assert.equal(payload.claude_sandbox, true);
  assert.equal(payload.unsafe_automation, undefined);
});

test("claude-code unsafe preset uses unsafe_automation", () => {
  const payload = buildRunSafetyPayload("claude-code", "code", "unsafe");
  assert.equal(payload.unsafe_automation, true);
  assert.equal(payload.claude_sandbox, true);
});
