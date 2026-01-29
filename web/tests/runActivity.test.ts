import test from "node:test";
import assert from "node:assert/strict";
import { deriveRunActivity } from "../src/runActivity.ts";

test("deriveRunActivity prefers meaningful log summaries over run.start noise", () => {
  const logs = [
    {
      time: "2026-01-29T00:00:01Z",
      stream: "system",
      message: 'run.start worker=claude-code dir="/tmp" cmd="claude" args=["-p","hi"]',
    },
    {
      time: "2026-01-29T00:00:02Z",
      stream: "assistant",
      message: JSON.stringify({
        type: "assistant",
        message: {
          role: "assistant",
          content: [{ type: "tool_use", name: "bash", id: "call_abcdef0123456789" }],
        },
      }),
    },
  ];

  const act = deriveRunActivity(logs);
  assert.ok(act);
  assert.equal(act.stream, "assistant");
  assert.match(act.summary, /tool_use bash/);
});

test("deriveRunActivity falls back to newest log when only noise exists", () => {
  const logs = [
    { time: "2026-01-29T00:00:01Z", stream: "system", message: "run.start a" },
    { time: "2026-01-29T00:00:03Z", stream: "system", message: "run.start b" },
  ];

  const act = deriveRunActivity(logs);
  assert.ok(act);
  assert.equal(act.time, "2026-01-29T00:00:03Z");
});

