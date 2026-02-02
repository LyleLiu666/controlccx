import assert from "node:assert/strict";
import test from "node:test";

import { shouldSkipAutoDeliveryForemanForTask } from "../src/deliveryForeman.ts";

test("shouldSkipAutoDeliveryForemanForTask skips blocked runs", () => {
  assert.equal(shouldSkipAutoDeliveryForemanForTask({ status: "blocked", error: "", warning: "" } as any), true);
});

test("shouldSkipAutoDeliveryForemanForTask skips no-conversation-found failures", () => {
  const base = { status: "failed", error: "", warning: "" } as any;
  assert.equal(shouldSkipAutoDeliveryForemanForTask({ ...base, error: "No conversation found with session ID: abc" }), true);
  assert.equal(
    shouldSkipAutoDeliveryForemanForTask({ ...base, warning: "resume failed: No conversation found with session ID: abc" }),
    true,
  );
});

test("shouldSkipAutoDeliveryForemanForTask does not skip normal failures/success", () => {
  assert.equal(shouldSkipAutoDeliveryForemanForTask({ status: "failed", error: "boom", warning: "" } as any), false);
  assert.equal(shouldSkipAutoDeliveryForemanForTask({ status: "succeeded", error: "", warning: "" } as any), false);
});
