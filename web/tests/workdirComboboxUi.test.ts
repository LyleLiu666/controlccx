import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Workdir input uses custom combobox (no native datalist arrow)", () => {
  const appVue = readText("../src/App.vue");
  const modalVue = readText("../src/components/NewRunModal.vue");

  assert.ok(
    appVue.includes("<WorkdirCombobox"),
    "App.vue should use WorkdirCombobox for the home workdir input",
  );
  assert.ok(
    modalVue.includes("<WorkdirCombobox"),
    "NewRunModal.vue should use WorkdirCombobox for the modal workdir input",
  );

  assert.equal(
    appVue.includes('list="workdirSuggestions"'),
    false,
    'App.vue should not rely on native datalist via list="workdirSuggestions"',
  );
  assert.equal(
    modalVue.includes('list="workdirSuggestions"'),
    false,
    'NewRunModal.vue should not rely on native datalist via list="workdirSuggestions"',
  );

  assert.equal(
    appVue.includes('<datalist id="workdirSuggestions">'),
    false,
    "App.vue should not render native <datalist> for workdir suggestions",
  );
});

