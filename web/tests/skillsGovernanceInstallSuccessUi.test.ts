import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("skills governance modal shows install success popup", () => {
  const modalVue = readText("../src/components/SkillsGovernanceModal.vue");

  assert.ok(modalVue.includes("noticeOpen"), "should track notice popup state");
  assert.ok(modalVue.includes("安装成功"), "should include install success copy");
  assert.ok(modalVue.includes("操作失败"), "should include error popup copy");
  assert.ok(modalVue.includes("TARGET_EXISTS|"), "should recognize target exists errors");
  assert.ok(modalVue.includes('@click="importExisting"'), "import action should be wrapped");
  assert.ok(modalVue.includes('@click="sync"'), "sync action should be wrapped");
  assert.ok(modalVue.includes('@click="updateFromSource"'), "update action should be wrapped");
  assert.ok(
    modalVue.includes("installLocal") || modalVue.includes("installGitSelected"),
    "should wire install actions through handlers",
  );
});
