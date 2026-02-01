import assert from "node:assert/strict";
import test from "node:test";

import { detectSkillInstallKind, normalizeGitRepoURL } from "../src/skillInstall.ts";

test("detectSkillInstallKind detects git URLs", () => {
  assert.equal(detectSkillInstallKind("https://github.com/user/repo"), "git");
  assert.equal(detectSkillInstallKind("ssh://git@github.com/user/repo"), "git");
  assert.equal(detectSkillInstallKind("git@github.com:user/repo.git"), "git");
  assert.equal(detectSkillInstallKind("https://example.com/foo/bar.git#main"), "git");
});

test("detectSkillInstallKind detects local paths", () => {
  assert.equal(detectSkillInstallKind("/Users/alice/skills/foo"), "local");
  assert.equal(detectSkillInstallKind("~/skills/foo"), "local");
  assert.equal(detectSkillInstallKind("./skills/foo"), "local");
  assert.equal(detectSkillInstallKind("../skills/foo"), "local");
  assert.equal(detectSkillInstallKind("C:\\\\skills\\\\foo"), "local");
  assert.equal(detectSkillInstallKind("file:///Users/alice/skills/foo"), "local");
});

test("detectSkillInstallKind returns unknown for ambiguous strings", () => {
  assert.equal(detectSkillInstallKind("foo"), "unknown");
  assert.equal(detectSkillInstallKind("foo/bar"), "unknown");
});

test("normalizeGitRepoURL keeps full URLs unchanged", () => {
  assert.equal(normalizeGitRepoURL("https://github.com/user/repo"), "https://github.com/user/repo");
  assert.equal(normalizeGitRepoURL("git@github.com:user/repo.git"), "git@github.com:user/repo.git");
});

test("normalizeGitRepoURL expands owner/repo shorthand to GitHub", () => {
  assert.equal(normalizeGitRepoURL("user/repo"), "https://github.com/user/repo");
  assert.equal(normalizeGitRepoURL("user.repo/repo_2"), "https://github.com/user.repo/repo_2");
});

