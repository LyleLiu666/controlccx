import assert from "node:assert/strict";
import test from "node:test";

import { deriveNextSelectedTaskId } from "../src/taskSelection.ts";

test("deriveNextSelectedTaskId respects autoSelectFirst for empty selection", () => {
  const list = [{ id: "a" }, { id: "b" }];

  assert.equal(
    deriveNextSelectedTaskId({ current: "", candidates: list, autoSelectFirst: true }),
    "a",
  );
  assert.equal(
    deriveNextSelectedTaskId({ current: "", candidates: list, autoSelectFirst: false }),
    "",
  );
  assert.equal(
    deriveNextSelectedTaskId({ current: "   ", candidates: list, autoSelectFirst: false }),
    "",
  );
});

test("deriveNextSelectedTaskId keeps valid selection and heals invalid selection", () => {
  const list = [{ id: "a" }, { id: "b" }];

  assert.equal(
    deriveNextSelectedTaskId({ current: "b", candidates: list, autoSelectFirst: false }),
    "b",
  );
  assert.equal(
    deriveNextSelectedTaskId({ current: "missing", candidates: list, autoSelectFirst: false }),
    "a",
  );
  assert.equal(
    deriveNextSelectedTaskId({ current: "missing", candidates: [], autoSelectFirst: false }),
    "",
  );
});

