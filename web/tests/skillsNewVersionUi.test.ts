import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Skills panel shows a non-misleading new-version indicator (narrow layout safe)", () => {
  const panel = readText("../src/components/SkillsPanel.vue");
  const css = readText("../src/App.css");

  assert.match(panel, /新版本/);
  assert.match(panel, /s\.new_version/);
  assert.match(panel, /不自动切换/);

  assert.match(css, /@container \(max-width: 520px\)[\s\S]*?\.skillsNameTop/);
  assert.match(css, /@container \(max-width: 520px\)[\s\S]*?\.skillsNameTop[\s\S]*flex-wrap:\s*wrap/);
});
