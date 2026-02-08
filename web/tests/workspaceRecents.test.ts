import assert from "node:assert/strict";
import test from "node:test";

import {
  MAX_WORKSPACE_RECENTS,
  listRecentWorkspacePaths,
  mergeWorkspaceRecents,
  rememberWorkspacePath,
} from "../src/workspaceRecents.ts";

function mkTask(partial: any) {
  return {
    id: partial.id ?? "t1",
    created_at: partial.created_at ?? "2026-02-01T00:00:00Z",
    workdir: partial.workdir ?? "/repo/default",
  } as any;
}

test("MAX_WORKSPACE_RECENTS is fixed at 20", () => {
  assert.equal(MAX_WORKSPACE_RECENTS, 20);
});

test("listRecentWorkspacePaths keeps only latest 20 unique workdirs", () => {
  const tasks = [] as any[];
  for (let i = 0; i < 26; i += 1) {
    const day = String(i + 1).padStart(2, "0");
    tasks.push(
      mkTask({
        id: `t${i}`,
        workdir: `/repo/${i}`,
        created_at: `2026-01-${day}T00:00:00Z`,
      }),
    );
  }

  const out = listRecentWorkspacePaths(tasks);
  assert.equal(out.length, 20);
  assert.equal(out[0], "/repo/25");
  assert.equal(out[19], "/repo/6");
});

test("rememberWorkspacePath moves selected workspace to front and keeps 20", () => {
  const seed = Array.from({ length: 20 }, (_, i) => `/repo/${i}`);
  const out = rememberWorkspacePath(seed, "/repo/3");
  assert.equal(out.length, 20);
  assert.equal(out[0], "/repo/3");
  assert.equal(out.filter((x) => x === "/repo/3").length, 1);
});

test("mergeWorkspaceRecents de-duplicates equivalent paths and caps at 20", () => {
  const fromLocal = ["/repo/a", "/repo/b/"];
  const fromTasks = ["/repo/c", "/repo/b", "C:\\repo\\d"];
  const out = mergeWorkspaceRecents(fromLocal, fromTasks);

  assert.deepEqual(out.slice(0, 4), ["/repo/a", "/repo/b/", "/repo/c", "C:\\repo\\d"]);

  const filled = mergeWorkspaceRecents(
    out,
    Array.from({ length: 30 }, (_, i) => `/repo/x/${i}`),
  );
  assert.equal(filled.length, 20);

  const pinnedSized = mergeWorkspaceRecents(
    Array.from({ length: 30 }, (_, i) => `/repo/p/${i}`),
    [],
    12,
  );
  assert.equal(pinnedSized.length, 12);
});
