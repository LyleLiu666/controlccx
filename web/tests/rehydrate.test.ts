import assert from "node:assert/strict";
import test from "node:test";

import { shouldOfferRehydrateForTask } from "../src/rehydrate.ts";

test("shouldOfferRehydrateForTask offers rehydrate for manual/unknown claude resume missing-session failures", () => {
  const base = {
    worker_type: "claude-code",
    mode: "resume",
    status: "failed",
    error: "No conversation found with session ID: abc",
  } as any;

  assert.equal(shouldOfferRehydrateForTask(base, "manual"), true);
  assert.equal(shouldOfferRehydrateForTask(base, ""), true);
  assert.equal(shouldOfferRehydrateForTask({ ...base, status: "succeeded" }, "manual"), false);
  assert.equal(shouldOfferRehydrateForTask({ ...base, mode: "new" }, "manual"), false);
  assert.equal(shouldOfferRehydrateForTask({ ...base, worker_type: "codex" }, "manual"), false);
  assert.equal(shouldOfferRehydrateForTask({ ...base, error: "boom" }, "manual"), false);
});

test("shouldOfferRehydrateForTask also matches warning field", () => {
  const base = {
    worker_type: "claude-code",
    mode: "resume",
    status: "failed",
    error: "",
    warning: "resume failed: No conversation found with session ID: abc",
  } as any;

  assert.equal(shouldOfferRehydrateForTask(base, "manual"), true);
  assert.equal(shouldOfferRehydrateForTask(base, ""), true);
});
