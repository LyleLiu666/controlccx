import assert from "node:assert/strict";
import test from "node:test";

import {
  ATTENTION_AUTOPILOT_STOP_VALUE,
  attentionAutopilotIsNoConversationFound,
  attentionAutopilotMarkSeen,
  attentionAutopilotSeenAtMs,
  attentionAutopilotShouldAttempt,
  attentionAutopilotStopForSession,
} from "../src/attentionAutopilot.ts";

test("attentionAutopilotSeenAtMs parses timestamps and stop sentinel", () => {
  assert.equal(attentionAutopilotSeenAtMs({}, "s:1"), 0);
  assert.equal(attentionAutopilotSeenAtMs({ "s:1": "123" }, "s:1"), 123);
  assert.equal(
    attentionAutopilotSeenAtMs({ "s:1": ATTENTION_AUTOPILOT_STOP_VALUE }, "s:1"),
    Number.POSITIVE_INFINITY,
  );
});

test("attentionAutopilotMarkSeen writes nowMs", () => {
  const next = attentionAutopilotMarkSeen({}, "s:1", 777);
  assert.deepEqual(next, { "s:1": "777" });
});

test("attentionAutopilotStopForSession writes stop sentinel", () => {
  const next = attentionAutopilotStopForSession({ "s:1": "1" }, "s:1");
  assert.deepEqual(next, { "s:1": ATTENTION_AUTOPILOT_STOP_VALUE });
});

test("attentionAutopilotShouldAttempt respects gating and cooldown", () => {
  const base = {
    enabled: true,
    deleted: false,
    hasSessionID: true,
    sessionStatus: "interrupted",
    latestStatus: "failed",
    nowMs: 1000,
    lastAttemptMs: 0,
    cooldownMs: 300000,
  };

  assert.equal(attentionAutopilotShouldAttempt(base), true);
  assert.equal(
    attentionAutopilotShouldAttempt({ ...base, sessionStatus: "failed" }),
    false,
  );
  assert.equal(attentionAutopilotShouldAttempt({ ...base, deleted: true }), false);
  assert.equal(
    attentionAutopilotShouldAttempt({ ...base, latestStatus: "running" }),
    false,
  );
  assert.equal(
    attentionAutopilotShouldAttempt({ ...base, latestStatus: "awaiting_approval" }),
    false,
  );
  assert.equal(
    attentionAutopilotShouldAttempt({ ...base, latestStatus: "blocked" }),
    false,
  );
  assert.equal(
    attentionAutopilotShouldAttempt({ ...base, lastAttemptMs: 900 }),
    false,
  );
  assert.equal(
    attentionAutopilotShouldAttempt({
      ...base,
      lastAttemptMs: Number.POSITIVE_INFINITY,
    }),
    false,
  );
});

test("attentionAutopilotIsNoConversationFound detects resume-missing sessions", () => {
  assert.equal(attentionAutopilotIsNoConversationFound(""), false);
  assert.equal(attentionAutopilotIsNoConversationFound("boom"), false);
  assert.equal(
    attentionAutopilotIsNoConversationFound(
      "No conversation found with session ID: abc",
    ),
    true,
  );
});
