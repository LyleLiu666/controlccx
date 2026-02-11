import assert from "node:assert/strict";
import test from "node:test";

import { buildSkillsRepoView } from "../src/skillsRepoGrouping.ts";

test("grouped mode hides non-git skills and sorts groups/items ascending", () => {
  const view = buildSkillsRepoView({
    skills: [
      { name: "z-local" },
      { name: "b-skill", repo_key: "github.com/acme/repo-b", repo_label: "acme/repo-b" },
      { name: "a-skill", repo_key: "github.com/acme/repo-a", repo_label: "acme/repo-a" },
      { name: "c-skill", repo_key: "github.com/acme/repo-a", repo_label: "acme/repo-a" },
    ] as any,
    q: "",
    repo: "",
    groupByRepo: true,
  });

  assert.equal(view.items.length, 3);
  assert.equal(view.groups.length, 2);
  assert.equal(view.groups[0].label, "acme/repo-a");
  assert.equal(view.groups[1].label, "acme/repo-b");
  assert.deepEqual(
    view.groups[0].skills.map((s) => s.name),
    ["a-skill", "c-skill"],
  );
});

test("repo filter and q use AND semantics", () => {
  const view = buildSkillsRepoView({
    skills: [
      { name: "alpha-one", repo_key: "github.com/acme/repo", repo_label: "acme/repo" },
      { name: "alpha-two", repo_key: "github.com/acme/repo", repo_label: "acme/repo" },
      { name: "beta-two", repo_key: "github.com/acme/repo-b", repo_label: "acme/repo-b" },
    ] as any,
    q: "two",
    repo: "github.com/acme/repo",
    groupByRepo: false,
  });

  assert.equal(view.items.length, 1);
  assert.equal(view.items[0].name, "alpha-two");
});

test("grouped empty flag is true when grouped mode has no git skills", () => {
  const view = buildSkillsRepoView({
    skills: [{ name: "local-only" }] as any,
    q: "",
    repo: "",
    groupByRepo: true,
  });

  assert.equal(view.items.length, 0);
  assert.equal(view.groupedEmpty, true);
});
