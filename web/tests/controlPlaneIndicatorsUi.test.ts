import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("UI exposes control-plane health and degraded states", () => {
  const appVue = readText("../src/App.vue");
  const api = readText("../src/api.ts");
  const types = readText("../src/types.ts");
  const composable = readText("../src/composables/useControlPlaneHealth.ts");
  const drawer = readText("../src/components/SecretaryDrawer.vue");
  const css = readText("../src/App.css");

  assert.match(types, /export type ControlPlaneStatus/);
  assert.match(api, /fetchControlPlaneStatus/);
  assert.match(api, /\/api\/control-plane/);
  assert.match(composable, /useControlPlaneHealth/);
  assert.match(composable, /fetchControlPlaneStatus/);

  assert.match(appVue, /controlPlanePills/);
  assert.match(appVue, /taskPlaneDegraded/);
  assert.match(appVue, /secretaryDegraded/);

  assert.match(drawer, /secretaryAvailable/);
  assert.match(drawer, /secDegradedHint/);

  assert.match(css, /\.controlPlanePills/);
  assert.match(css, /\.banner\.warn/);
});

