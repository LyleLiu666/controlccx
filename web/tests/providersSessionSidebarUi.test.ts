import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers page can show Sessions panel in the same grid", () => {
  const appVue = readText("../src/App.vue");

  assert.match(
    appVue,
    /const sessionsHostAllowsPanel = computed\([\s\S]*!skillsOpen\.value\s*&&\s*!contextOpen\.value\s*&&\s*!filesOpen\.value[\s\S]*\);/,
  );
  assert.match(
    appVue,
    /const showSessionsPanel = computed\([\s\S]*sessionsHostAllowsPanel\.value\s*&&\s*sessionsDrawerOpen\.value[\s\S]*\);/,
  );
  assert.match(appVue, /<div class="grid" :class="\{ gridSingle: !showSessionsPanel \}">/);

  assert.match(
    appVue,
    /<template v-else>[\s\S]*<section\s+v-if="showSessionsPanel"[\s\S]*<ProvidersPanel\s+v-if="providersSettingsOpen"/,
  );
});

test("Opening Providers does not force-close Sessions drawer", () => {
  const appVue = readText("../src/App.vue");

  const openProvidersSettingsBlock =
    appVue.match(
      /function openProvidersSettings\(\)\s*\{[\s\S]*?providersSettingsOpen\.value = true;[\s\S]*?\n\}/,
    )?.[0] ?? "";
  assert.ok(openProvidersSettingsBlock.includes("function openProvidersSettings()"));
  assert.ok(
    !openProvidersSettingsBlock.includes("sessionsDrawerOpen.value = false;"),
    "openProvidersSettings should not reset sessions drawer",
  );

  const routeProvidersBlock =
    appVue.match(
      /if \(path === "\/providers"\)\s*\{[\s\S]*?providersSettingsOpen\.value = true;[\s\S]*?return;\n\s*\}/,
    )?.[0] ?? "";
  assert.ok(routeProvidersBlock.includes('if (path === "/providers")'));
  assert.ok(
    !routeProvidersBlock.includes("sessionsDrawerOpen.value = false;"),
    "route handling for /providers should preserve sessions drawer state",
  );
});
