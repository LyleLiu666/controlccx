import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Providers settings modal hides env import hint and keeps speed test actions", () => {
  const panel = readText("../src/components/ProvidersPanel.vue");
  assert.match(panel, /authStatus/);
  assert.ok(!panel.includes("检测到可导入的环境变量"));
  assert.ok(!panel.includes("环境变量仅用于辅助填充，不会覆盖已保存配置"));
  assert.ok(!panel.includes("检测到环境变量覆盖"));
  assert.match(panel, />\s*速度测试\s*</);
  assert.match(panel, /emit\('speedtest'/);
});

test("App wires Providers status and speed test handlers", () => {
  const appVue = readText("../src/App.vue");
  assert.match(appVue, /:authStatus=\"authStatus\"/);
  assert.match(appVue, /@speedtest=\"runProviderSpeedTest\"/);
});
