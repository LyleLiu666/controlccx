import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page defines dedicated layout classes for robust scrolling", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /class="[^"]*providersPage/);
  assert.match(panel, /class="providersHeader"/);
  assert.match(panel, /class="providersBody"/);
  assert.match(panel, /class="[^"]*providersSplit/);
  assert.match(panel, /class="[^"]*providersNav/);
  assert.match(panel, /class="[^"]*providersEditor/);
});

test("Providers page scoped styles keep body/nav/editor scrollable", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(
    panel,
    /\.providersBody\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;[^}]*min-height:\s*0;[^}]*overflow:\s*auto;/s,
  );
  assert.match(
    panel,
    /\.providersSplit\s*\{[^}]*flex:\s*1;[^}]*min-height:\s*0;/s,
  );
  assert.match(panel, /\.providersNav\s*\{[^}]*overflow:\s*auto;/s);
  assert.match(panel, /\.providersEditor\s*\{[^}]*overflow:\s*auto;/s);
});
