import assert from "node:assert/strict";
import test from "node:test";

import { isNoConversationFound } from "../src/noConversationFound.ts";

test("isNoConversationFound detects missing-session errors", () => {
  assert.equal(isNoConversationFound(""), false);
  assert.equal(isNoConversationFound("boom"), false);
  assert.equal(isNoConversationFound("No conversation found with session ID: abc"), true);
  assert.equal(isNoConversationFound("no conversation found with SESSION id: abc"), true);
  assert.equal(isNoConversationFound("No conversation found with conversation ID: abc"), false);
});

