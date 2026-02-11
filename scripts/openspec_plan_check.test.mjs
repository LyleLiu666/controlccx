import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { computeReadySet, findChangeDir, isChangeCompleted, validateManifest } from "./openspec_plan_check.mjs";

test("validateManifest accepts acyclic ordered dependency graph", () => {
  const manifest = {
    layers: ["T0", "T1", "T2"],
    changes: [
      { id: "a", layer: "T0", hard_dependencies: [] },
      { id: "b", layer: "T1", hard_dependencies: ["a"] },
      { id: "c", layer: "T2", hard_dependencies: ["b"] },
    ],
  };
  const out = validateManifest(manifest, process.cwd());
  assert.equal(out.ok, false); // files are missing in cwd, but graph should not add cycle/layer errors
  assert.equal(out.errors.some((e) => e.includes("cycle detected")), false);
  assert.equal(out.errors.some((e) => e.includes("later layer")), false);
});

test("validateManifest detects cycle", () => {
  const manifest = {
    layers: ["T0", "T1"],
    changes: [
      { id: "a", layer: "T0", hard_dependencies: ["b"] },
      { id: "b", layer: "T1", hard_dependencies: ["a"] },
    ],
  };
  const out = validateManifest(manifest, process.cwd());
  assert.equal(out.errors.some((e) => e.includes("cycle detected")), true);
});

test("validateManifest detects dependency in later layer", () => {
  const manifest = {
    layers: ["T0", "T1"],
    changes: [
      { id: "a", layer: "T0", hard_dependencies: ["b"] },
      { id: "b", layer: "T1", hard_dependencies: [] },
    ],
  };
  const out = validateManifest(manifest, process.cwd());
  assert.equal(out.errors.some((e) => e.includes("later layer")), true);
});

test("isChangeCompleted requires all checklist items checked", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ccx-open-spec-"));
  const file = path.join(dir, "tasks.md");
  fs.writeFileSync(file, "## 1\n- [x] done\n- [x] done 2\n");
  assert.equal(isChangeCompleted(file), true);
  fs.writeFileSync(file, "## 1\n- [x] done\n- [ ] todo\n");
  assert.equal(isChangeCompleted(file), false);
});

test("computeReadySet returns only nodes whose hard deps are completed", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "ccx-open-spec-repo-"));
  const manifest = {
    layers: ["T0", "T1", "T2"],
    changes: [
      { id: "a", layer: "T0", hard_dependencies: [] },
      { id: "b", layer: "T1", hard_dependencies: ["a"] },
      { id: "c", layer: "T2", hard_dependencies: ["b"] },
    ],
  };
  for (const id of ["a", "b", "c"]) {
    const d = path.join(repo, "openspec", "changes", id);
    fs.mkdirSync(d, { recursive: true });
    fs.writeFileSync(path.join(d, "tasks.md"), "## 1\n- [ ] todo\n");
  }
  fs.writeFileSync(path.join(repo, "openspec", "changes", "a", "tasks.md"), "## 1\n- [x] done\n");

  const out = computeReadySet(manifest, repo);
  assert.deepEqual(out.completed, ["a"]);
  assert.equal(out.ready.some((x) => x.id === "b"), true);
  assert.equal(out.ready.some((x) => x.id === "c"), false);
});

test("findChangeDir resolves archived changes", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "ccx-open-spec-archive-"));
  const archived = path.join(repo, "openspec", "changes", "archive", "2026-02-11-add-x");
  fs.mkdirSync(archived, { recursive: true });
  fs.writeFileSync(path.join(archived, "proposal.md"), "p\n");
  fs.writeFileSync(path.join(archived, "tasks.md"), "## 1\n- [x] done\n");

  const out = findChangeDir(repo, "add-x");
  assert.equal(Boolean(out), true);
  assert.equal(out?.archived, true);
  assert.equal(out?.dir, archived);
});
