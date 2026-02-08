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
  const css = readText("../src/App.css");

  assert.match(types, /export type ControlPlaneStatus/);
  assert.match(api, /fetchControlPlaneStatus/);
  assert.match(api, /\/api\/control-plane/);
  assert.match(composable, /useControlPlaneHealth/);
  assert.match(composable, /fetchControlPlaneStatus/);

  assert.match(appVue, /controlPlanePills/);
  assert.match(appVue, /taskPlaneDegraded/);
  assert.ok(!appVue.includes("secretaryd"));
  assert.ok(!types.includes("secretaryd"));
  assert.ok(!composable.includes("secretaryOK"));

  assert.match(css, /\.controlPlanePills/);
  assert.match(css, /\.banner\.warn/);
});
