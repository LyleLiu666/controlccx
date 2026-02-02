import assert from "node:assert/strict";
import test from "node:test";

import { summarizeSkillTarget } from "../src/skillsSummary.ts";

test("summarizeSkillTarget disables enable when source missing", () => {
  const summary = summarizeSkillTarget(
    {
      name: "threejs-materials",
      source: "",
      targets: [{ target: "cursor", root: "/tmp/cursor/skills", status: "missing" }],
    } as any,
    "cursor",
  );
  assert.equal(summary.status, "missing");
  assert.equal(summary.canEnable, false);
});

test("summarizeSkillTarget enables enable when source exists and no blockers", () => {
  const summary = summarizeSkillTarget(
    {
      name: "code-review-excellence",
      source: "/Users/demo/.agent/skills/code-review-excellence",
      targets: [{ target: "cursor", root: "/tmp/cursor/skills", status: "missing" }],
    } as any,
    "cursor",
  );
  assert.equal(summary.status, "missing");
  assert.equal(summary.canEnable, true);
});

test("summarizeSkillTarget disables enable when target has unmanaged entry", () => {
  const summary = summarizeSkillTarget(
    {
      name: "code-review-excellence",
      source: "/Users/demo/.agent/skills/code-review-excellence",
      targets: [{ target: "cursor", root: "/tmp/cursor/skills", status: "present" }],
    } as any,
    "cursor",
  );
  assert.equal(summary.status, "present");
  assert.equal(summary.canEnable, false);
});
