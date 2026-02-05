import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Workdir picker supports recent dropdown + typing (New Run modal + Home)", () => {
  const appVue = readText("../src/App.vue");
  const newRunModal = readText("../src/components/NewRunModal.vue");

  // A shared datalist provides recent directories while keeping free-typing.
  assert.match(appVue, /<datalist\s+id="workdirSuggestions">/);
  assert.match(appVue, /v-for="p in workdirSuggestions"/);

  // Home form workdir input uses the shared suggestions.
  assert.match(
    appVue,
    /<input[^>]*v-model="newWorkdir"[^>]*\slist="workdirSuggestions"/,
  );

  // NewRunModal workdir input uses the shared suggestions.
  assert.match(newRunModal, /\slist="workdirSuggestions"/);
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

