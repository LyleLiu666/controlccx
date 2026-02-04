import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("sessions list surfaces a Secretary review status to avoid surprising auto-continue", () => {
  const appVue = readText("../src/App.vue");

  assert.ok(
    appVue.includes('class="pill review"'),
    "expected a visible review pill in session rows",
  );
  assert.ok(
    appVue.includes("秘书审阅中"),
    "expected copy to mention secretary review in progress",
  );
  assert.ok(
    appVue.includes("秘书审阅排队"),
    "expected copy to mention secretary review queued",
  );
});

