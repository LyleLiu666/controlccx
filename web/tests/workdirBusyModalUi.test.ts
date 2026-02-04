import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("workdir busy modal exists and is wired in App.vue", () => {
  const appVue = readText("../src/App.vue");
  const modalVue = readText("../src/components/WorkdirBusyModal.vue");

  assert.ok(appVue.includes("WorkdirBusyModal"));
  assert.ok(appVue.includes("<WorkdirBusyModal"));
  assert.ok(
    appVue.includes('@wait="confirmWorkdirBusyWait"') ||
      appVue.includes("@wait=\"confirmWorkdirBusyWait\""),
    "WorkdirBusyModal should wire @wait handler",
  );
  assert.ok(
    appVue.includes('@worktree="confirmWorkdirBusyWorktree"') ||
      appVue.includes("@worktree=\"confirmWorkdirBusyWorktree\""),
    "WorkdirBusyModal should wire @worktree handler",
  );
  assert.ok(appVue.includes("extractWorkdirBusyPayload("));
  assert.ok(appVue.includes('confirmWorkdirBusyStrategy("worktree")'));
  assert.ok(appVue.includes('confirmWorkdirBusyStrategy("wait")'));

  assert.ok(modalVue.includes("工作目录被占用"));
  assert.ok(modalVue.includes("等待（排队）"));
  assert.ok(modalVue.includes("创建 Worktree"));
});

