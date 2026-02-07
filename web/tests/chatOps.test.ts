import assert from "node:assert/strict";
import test from "node:test";

import { appendChatMessageUnique, buildChatIdempotencyKey, sendChatAndReload } from "../src/chatOps.ts";

test("appendChatMessageUnique appends and dedups by id", () => {
  const list = [
    { id: 1, time: "t1", role: "user", content: "hello" },
    { id: 2, time: "t2", role: "assistant", content: "hi" },
  ];

  const dup = { id: 2, time: "t2b", role: "assistant", content: "ignored" };
  const same = appendChatMessageUnique(list, dup as any);
  assert.equal(same, list);
  assert.equal(same.length, 2);

  const fresh = { id: 3, time: "t3", role: "user", content: "next" };
  const next = appendChatMessageUnique(list, fresh as any);
  assert.notEqual(next, list);
  assert.equal(next.length, 3);
  assert.deepEqual(next[2], fresh);
});

test("sendChatAndReload calls sendChat then fetchChat", async () => {
  const calls: any[] = [];

  const sendChat = async (message: string) => {
    calls.push(["sendChat", message]);
    return { message: "ok" };
  };

  const fetchChat = async (after?: number, limit?: number) => {
    calls.push(["fetchChat", after, limit]);
    return [{ id: 1, time: "t1", role: "assistant", content: "ok" }] as any[];
  };

  const res = await sendChatAndReload("hello", { sendChat, fetchChat, after: 0, limit: 200 });
  assert.deepEqual(calls, [
    ["sendChat", "hello"],
    ["fetchChat", 0, 200],
  ]);
  assert.equal(res.length, 1);
});

test("buildChatIdempotencyKey changes across chat history (after)", () => {
  const a = buildChatIdempotencyKey({ after: 10, message: "ping", backend: "auto", maxSteps: 8 });
  const b = buildChatIdempotencyKey({ after: 11, message: "ping", backend: "auto", maxSteps: 8 });
  assert.notEqual(a, b);
  assert.match(a, /^chat:10:/);
  assert.match(b, /^chat:11:/);
});

test("buildChatIdempotencyKey is stable for same inputs", () => {
  const a = buildChatIdempotencyKey({ after: 0, message: "ping", backend: "auto", maxSteps: 8 });
  const b = buildChatIdempotencyKey({ after: 0, message: " ping ", backend: "auto", maxSteps: 8 });
  assert.equal(a, b);
});
