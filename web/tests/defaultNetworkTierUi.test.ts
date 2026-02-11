import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("new run shows selected network tier and create payload carries default web_readonly", () => {
  const modal = readText("../src/components/NewRunModal.vue");
  const appVue = readText("../src/App.vue");

  assert.ok(
    modal.includes("网络档位"),
    "NewRunModal should show the selected network tier to the user",
  );
  assert.ok(
    appVue.includes("network_tier: safety.network_tier ?? defaultNetworkTier"),
    "onCreateTask should set network_tier in create payload (default web_readonly when autopilot path is used)",
  );
});
