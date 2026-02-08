import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const appVuePath = resolve(here, "../src/App.vue");
const appVue = readFileSync(appVuePath, "utf8");

test("sessions list ordering is delegated to backend (no frontend sort call)", () => {
  assert.ok(
    !appVue.includes("out.sort(compareByLatestRunDesc)"),
    "sessions ordering should not call compareByLatestRunDesc in App.vue",
  );
  assert.ok(
    !appVue.includes('from "./sessionOrdering"'),
    "App.vue should not import sessionOrdering helper for sessions list ordering",
  );
});
