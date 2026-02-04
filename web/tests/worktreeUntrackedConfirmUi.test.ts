import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Worktree untracked confirmation UI exists (skip/force) and is wired", () => {
  const app = readText("../src/App.vue");
  assert.ok(app.includes("worktree_untracked_too_large"));
  assert.ok(app.includes("worktree_untracked"));

  const modal = readText("../src/components/WorktreeUntrackedModal.vue");
  assert.ok(modal.includes("继续但不复制 untracked"));
  assert.ok(modal.includes("仍然复制 untracked"));
  assert.ok(modal.includes("node_modules"));
  assert.ok(modal.includes(".venv"));
});

