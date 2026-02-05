import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Workdir picker supports recent dropdown + typing (New Run modal + Home)", () => {
  const appVue = readText("../src/App.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");
  const combobox = readText("../src/components/WorkdirCombobox.vue");

  // Home form workdir input keeps free typing but adds a custom suggestions menu.
  assert.ok(appVue.includes("<WorkdirCombobox"));
  assert.ok(appVue.includes('v-model="newWorkdir"'));
  assert.ok(appVue.includes(':pinned="workdirPinnedOptions"'));
  assert.ok(appVue.includes(':recent="workdirRecentOptions"'));

  // NewRunModal workdir input uses the same combobox pattern.
  assert.ok(newRunModal.includes("<WorkdirCombobox"));
  assert.ok(newRunModal.includes(':modelValue="workdir"'));
  assert.ok(
    newRunModal.includes(`@update:modelValue="emit('update:workdir', $event)"`),
  );

  // The combobox itself is built around an <input> (typing) + a toggle button (dropdown).
  assert.ok(combobox.includes("@input=\"onInput\""));
  assert.ok(combobox.includes('class="workdirComboToggle"'));
});

test("New Run modal spacing avoids cramped skills hint / advanced menu", () => {
  const css = readText("../src/App.css");

  // Skills hint should have breathing room before the next section (e.g. Advanced).
  assert.match(
    css,
    /:deep\(\.newRunSkillsHint\)\s*{[^}]*margin-bottom:\s*\d+px[^}]*}/,
  );

  // Avoid putting the skills button on the far right (looks bulky in wide dialogs).
  assert.match(
    css,
    /:deep\(\.newRunPromptLabelRow\)\s*{[^}]*justify-content:\s*flex-start[^}]*}/,
  );
});
