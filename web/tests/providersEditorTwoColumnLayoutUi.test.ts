import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers editor marks auth/model sections for two-column placement", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /class="providersSubsection providersSubsectionAuth"/);
  assert.match(panel, /class="providersSubsection providersSubsectionModel"/);
});

test("Providers editor styles switch to desktop two-column and keep core rows full width", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /\.providersEditorGrid\s*>\s*\.full\s*\{[^}]*grid-column:\s*1\s*\/\s*-1;/s);
  assert.match(panel, /@media\s*\(min-width:\s*1040px\)\s*\{[\s\S]*\.providersEditorGrid\s*\{[\s\S]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+minmax\(0,\s*1fr\);/s);
  assert.match(panel, /\.providersSubsectionAuth\s*\{[^}]*grid-column:\s*1;/s);
  assert.match(panel, /\.providersSubsectionModel\s*\{[^}]*grid-column:\s*2;/s);
});
