import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("session header mini actions use Chinese labels", () => {
  const appVue = readText("../src/App.vue");

  assert.ok(appVue.includes('title="查看运行记录"'));
  assert.ok(appVue.includes("运行记录（{{ selectedSession.runs.length }}）"));

  assert.ok(appVue.includes('title="浏览工作区文件"'));
  assert.match(appVue, /title="浏览工作区文件"[\s\S]*>\s*文件\s*<\/button>/);

  assert.ok(appVue.includes("{{ s.runs.length }} 次运行"));
  assert.ok(appVue.includes("最后运行："));
});
