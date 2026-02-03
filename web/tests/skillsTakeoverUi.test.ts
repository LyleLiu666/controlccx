import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Skills panel exposes a takeover action for unmanaged targets", () => {
  const panel = readText("../src/components/SkillsPanel.vue");
  assert.match(panel, /接管|纳管/);
  assert.match(panel, /takeover/);
});

