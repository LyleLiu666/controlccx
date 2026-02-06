import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers modal defines dedicated layout classes for robust scrolling", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.match(modal, /class="modal toolsModal providersModal"/);
  assert.match(modal, /class="modalHeader providersHeader"/);
  assert.match(modal, /class="modalBody toolsBody providersBody"/);
  assert.match(modal, /class="toolsSplit providersSplit"/);
  assert.match(modal, /class="toolsList providersList"/);
  assert.match(modal, /class="toolsEditor providersEditor"/);
});

test("Providers modal scoped styles keep body/list/editor scrollable", () => {
  const modal = readText("../src/components/ProvidersSettingsModal.vue");
  assert.match(
    modal,
    /\.providersBody\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;[^}]*min-height:\s*0;[^}]*overflow:\s*auto;/s,
  );
  assert.match(
    modal,
    /\.providersSplit\s*\{[^}]*flex:\s*1;[^}]*min-height:\s*0;/s,
  );
  assert.match(modal, /\.providersList\s*\{[^}]*overflow:\s*auto;/s);
  assert.match(modal, /\.providersEditor\s*\{[^}]*overflow:\s*auto;/s);
});
