import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("high-permission warnings explain granted permissions (not alarmist)", () => {
  const appVue = readText("../src/App.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");
  const blockedPromptModal = readText("../src/components/BlockedPromptModal.vue");

  assert.ok(
    newRunModal.includes("开启下载/安装权限"),
    "NewRunModal should describe Install unlock as enabling download/install permissions",
  );
  assert.ok(
    newRunModal.includes("我已知晓将开放的权限"),
    "NewRunModal opt-in copy should be permission-focused",
  );

  assert.ok(
    appVue.includes("高权限确认"),
    "High-risk confirmation dialog should be framed as higher permissions",
  );
  assert.ok(
    blockedPromptModal.includes("高权限继续"),
    "Blocked-run recovery CTA should be framed as higher permissions",
  );
  assert.ok(
    blockedPromptModal.includes("保持当前安全设置重试"),
    "Blocked-run recovery should offer a safe retry CTA",
  );
  assert.ok(
    appVue.includes("需要开启下载/安装权限"),
    "Install unlock confirmation should be permission-focused",
  );
});
