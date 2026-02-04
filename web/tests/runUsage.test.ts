import test from "node:test";
import assert from "node:assert/strict";
import { deriveRunUsage } from "../src/runUsage.ts";

test("deriveRunUsage prefers latest result event (claude-code) and includes cost/models", () => {
  const logs = [
    {
      time: "2026-02-03T00:00:00Z",
      message: JSON.stringify({
        type: "turn.completed",
        usage: { input_tokens: 1, cached_input_tokens: 2, output_tokens: 3 },
      }),
    },
    {
      time: "2026-02-03T00:00:10Z",
      message: JSON.stringify({
        type: "result",
        total_cost_usd: 0.02,
        usage: {
          input_tokens: 1000,
          output_tokens: 200,
          cache_read_input_tokens: 300,
          cache_creation_input_tokens: 100,
        },
        modelUsage: {
          "claude-sonnet-x": {
            inputTokens: 1000,
            outputTokens: 200,
            cacheReadInputTokens: 300,
            cacheCreationInputTokens: 100,
            costUSD: 0.02,
            contextWindow: 200000,
            maxOutputTokens: 8192,
          },
        },
      }),
    },
  ];

  const usage = deriveRunUsage(logs);
  assert.ok(usage);
  assert.equal(usage.source, "result");
  assert.equal(usage.inputTokens, 1000);
  assert.equal(usage.outputTokens, 200);
  assert.equal(usage.cacheReadInputTokens, 300);
  assert.equal(usage.cacheCreationInputTokens, 100);
  assert.equal(usage.cachedInputTokens, 0);
  assert.equal(usage.inputTotalTokens, 1400);
  assert.equal(usage.totalTokens, 1600);
  assert.equal(usage.totalCostUSD, 0.02);
  assert.ok(usage.models?.length);
  assert.equal(usage.models?.[0]?.model, "claude-sonnet-x");
});

test("deriveRunUsage sums codex turn.completed usage when no result exists", () => {
  const logs = [
    {
      time: "2026-02-03T00:00:00Z",
      message: JSON.stringify({
        type: "turn.completed",
        usage: { input_tokens: 100, cached_input_tokens: 50, output_tokens: 10 },
      }),
    },
    {
      time: "2026-02-03T00:00:10Z",
      message: JSON.stringify({
        type: "turn.completed",
        usage: { input_tokens: 200, cached_input_tokens: 0, output_tokens: 20 },
      }),
    },
  ];

  const usage = deriveRunUsage(logs);
  assert.ok(usage);
  assert.equal(usage.source, "turn.completed");
  assert.equal(usage.inputTokens, 300);
  assert.equal(usage.cachedInputTokens, 50);
  assert.equal(usage.outputTokens, 30);
  assert.equal(usage.totalCostUSD, undefined);
  assert.equal(usage.inputTotalTokens, 350);
  assert.equal(usage.totalTokens, 380);
});

test("deriveRunUsage falls back to summing message.usage when needed", () => {
  const logs = [
    {
      time: "2026-02-03T00:00:00Z",
      message: JSON.stringify({
        type: "assistant",
        message: { role: "assistant", usage: { input_tokens: 5, output_tokens: 6 } },
      }),
    },
    {
      time: "2026-02-03T00:00:10Z",
      message: JSON.stringify({
        type: "assistant",
        message: { role: "assistant", usage: { input_tokens: 7, output_tokens: 8 } },
      }),
    },
  ];

  const usage = deriveRunUsage(logs);
  assert.ok(usage);
  assert.equal(usage.source, "message.usage");
  assert.equal(usage.inputTokens, 12);
  assert.equal(usage.outputTokens, 14);
  assert.equal(usage.totalTokens, 26);
});

