import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("prompt inputs opt into emphasis styling", () => {
  const newRunModal = readText("../src/components/NewRunModal.vue");
  const appVue = readText("../src/App.vue");
  const appCss = readText("../src/App.css");

  assert.ok(
    newRunModal.includes("promptEmphasis"),
    "NewRunModal prompt textarea should include the promptEmphasis class",
  );
  assert.ok(
    appVue.includes("promptEmphasis"),
    "Resume prompt input/textarea should include the promptEmphasis class",
  );

  assert.ok(
    appCss.includes("input.promptEmphasis") && appCss.includes("textarea.promptEmphasis"),
    "App.css should define promptEmphasis styles for both input and textarea",
  );
  assert.ok(
    appCss.includes("border-width: 2px"),
    "promptEmphasis should use a thicker border to improve affordance",
  );
  assert.ok(
    appCss.includes("box-shadow: var(--shadow-sm)"),
    "promptEmphasis should add a subtle outer shadow to stand out from surrounding UI",
  );
});

