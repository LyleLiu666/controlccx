import assert from "node:assert/strict";
import test from "node:test";

import { shouldDismissRunLaunchMask } from "../src/runLaunchMask.ts";

test("shouldDismissRunLaunchMask keeps mask while queued", () => {
  assert.equal(shouldDismissRunLaunchMask({ status: "queued" } as any), false);
  assert.equal(
    shouldDismissRunLaunchMask({ status: "queued", started_at: "" } as any),
    false,
  );
  assert.equal(shouldDismissRunLaunchMask({ status: "waiting" } as any), false);
  assert.equal(
    shouldDismissRunLaunchMask({ status: "waiting", started_at: "" } as any),
    false,
  );
});

test("shouldDismissRunLaunchMask dismisses once started or no longer queued", () => {
  assert.equal(shouldDismissRunLaunchMask({ status: "queued", started_at: "2026-01-01T00:00:00Z" } as any), true);
  assert.equal(shouldDismissRunLaunchMask({ status: "running" } as any), true);
  assert.equal(shouldDismissRunLaunchMask({ status: "succeeded" } as any), true);
  assert.equal(shouldDismissRunLaunchMask({ status: "failed" } as any), true);
  assert.equal(shouldDismissRunLaunchMask({ status: "blocked" } as any), true);
});
