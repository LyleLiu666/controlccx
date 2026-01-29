import test from "node:test";
import assert from "node:assert/strict";
import { prettifyLogMessage } from "../src/logPretty.ts";

test("prettifyLogMessage keeps non-json as text with truncated summary", () => {
  const out = prettifyLogMessage("hello world");
  assert.equal(out.kind, "text");
  assert.equal(out.summary, "hello world");
  assert.equal(out.details, "hello world");
});

test("prettifyLogMessage summarizes OpenAI-ish assistant tool_use JSON", () => {
  const raw = JSON.stringify({
    type: "assistant",
    message: {
      role: "assistant",
      model: "gpt-5",
      content: [{ type: "tool_use", name: "read_file", id: "call_abcdef0123456789" }],
    },
  });
  const out = prettifyLogMessage(raw);
  assert.equal(out.kind, "json");
  assert.match(out.summary, /tool_use read_file/);
  assert.match(out.summary, /call_abcdef01/i);
  assert.ok(out.prettyJson && out.prettyJson.includes('"type": "assistant"'));
});

test("prettifyLogMessage summarizes tool_result JSON", () => {
  const raw = JSON.stringify({
    type: "user",
    message: {
      role: "user",
      content: [{ type: "tool_result", tool_use_id: "call_12345678" }],
    },
  });
  const out = prettifyLogMessage(raw);
  assert.equal(out.kind, "json");
  assert.match(out.summary, /tool_result/);
  assert.match(out.summary, /call_12345678/);
});
