import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("approval prompt modal exists and is wired in App.vue", () => {
  const appVue = readText("../src/App.vue");
  const modalVue = readText("../src/components/ApprovalPromptModal.vue");

  assert.ok(appVue.includes("ApprovalPromptModal"));
  assert.ok(appVue.includes("<ApprovalPromptModal"));
  assert.ok(
    appVue.includes('@tokenRequired="openInstanceTokenModal"') ||
      appVue.includes("@tokenRequired=\"openInstanceTokenModal\""),
    "ApprovalPromptModal should wire @tokenRequired to openInstanceTokenModal",
  );
  assert.ok(
    appVue.includes('@enterUnsafe="onApprovalEnterUnsafe"') ||
      appVue.includes("@enterUnsafe=\"onApprovalEnterUnsafe\""),
    "ApprovalPromptModal should wire @enterUnsafe handler",
  );

  assert.ok(modalVue.includes("需要审批"));
  assert.ok(modalVue.includes("Deny"));
  assert.ok(modalVue.includes("Approve and continue"));
  assert.ok(modalVue.includes("Enter UNSAFE"));
  assert.ok(modalVue.includes("拒绝理由"));
});

