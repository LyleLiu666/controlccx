import assert from "node:assert/strict";
import test from "node:test";

import { buildRunSafetyPayload, toolDriverForWorkerType } from "../src/runSafety.ts";

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

test("toolDriverForWorkerType falls back to exec", () => {
  assert.equal(toolDriverForWorkerType(""), "exec");
  assert.equal(toolDriverForWorkerType("unknown"), "exec");
});

test("toolDriverForWorkerType resolves driver via tools list", () => {
  assert.equal(
    toolDriverForWorkerType("my-claude", [{ id: "my-claude", driver: "claude-code", command: "x" } as any]),
    "claude-code",
  );
  assert.equal(
    toolDriverForWorkerType("my-codex", [{ id: "my-codex", driver: "codex", command: "x" } as any]),
    "codex",
  );
});
