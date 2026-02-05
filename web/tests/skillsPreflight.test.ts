import assert from "node:assert/strict";
import test from "node:test";

import { buildSkillMountPlan } from "../src/skillsPreflight.ts";

test("buildSkillMountPlan returns null for unsupported drivers", () => {
  const plan = buildSkillMountPlan({
    driver: "exec" as any,
    prompt: "/code-review-excellence\nDo X\n",
    skills: [],
  });
  assert.equal(plan, null);
});

test("buildSkillMountPlan builds a plan for Codex when skill is mountable", () => {
  const plan = buildSkillMountPlan({
    driver: "codex",
    prompt: "/code-review-excellence\nDo X\n",
    skills: [
      {
        name: "code-review-excellence",
        source: "/Users/demo/.agent/skills/code-review-excellence",
        targets: [{ target: "codex", root: "/tmp/codex/skills", status: "missing" }],
      } as any,
    ],
  });
  assert.ok(plan);
  assert.equal(plan.target, "codex");
  assert.deepEqual(plan.namesToMount, ["code-review-excellence"]);
  assert.equal(plan.items[0]?.name, "code-review-excellence");
  assert.equal(plan.items[0]?.status, "missing");
});

test("buildSkillMountPlan builds a plan for Claude Code when skill is mountable", () => {
  const plan = buildSkillMountPlan({
    driver: "claude-code",
    prompt: "Use $code-review-excellence\n",
    skills: [
      {
        name: "code-review-excellence",
        source: "/Users/demo/.agent/skills/code-review-excellence",
        targets: [{ target: "claude_code", root: "/tmp/claude/skills", status: "missing" }],
      } as any,
    ],
  });
  assert.ok(plan);
  assert.equal(plan.target, "claude_code");
  assert.deepEqual(plan.namesToMount, ["code-review-excellence"]);
});

test("buildSkillMountPlan returns null when skill is already enabled", () => {
  const plan = buildSkillMountPlan({
    driver: "codex",
    prompt: "/code-review-excellence\n",
    skills: [
      {
        name: "code-review-excellence",
        source: "/Users/demo/.agent/skills/code-review-excellence",
        targets: [{ target: "codex", root: "/tmp/codex/skills", status: "linked" }],
      } as any,
    ],
  });
  assert.equal(plan, null);
});

test("buildSkillMountPlan returns null when skill cannot be enabled", () => {
  const plan = buildSkillMountPlan({
    driver: "codex",
    prompt: "/code-review-excellence\n",
    skills: [
      {
        name: "code-review-excellence",
        source: "",
        targets: [{ target: "codex", root: "/tmp/codex/skills", status: "missing" }],
      } as any,
    ],
  });
  assert.equal(plan, null);
});

