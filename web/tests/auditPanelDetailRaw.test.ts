import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

function readText(relativePath: string) {
  return readFileSync(new URL(relativePath, import.meta.url), "utf8");
}

test("Audit panel shows raw content by default in detail pane", () => {
  const panel = readText("../src/components/AuditPanel.vue");

  assert.match(panel, /<template v-else-if="detail">/);
  assert.match(panel, /<pre class="auditDetailRaw">\{\{ detail\.raw \}\}<\/pre>/);
  assert.match(panel, /<pre v-if="detail\.meta" class="auditDetailMeta">\{\{ JSON\.stringify\(detail\.meta, null, 2\) \}\}<\/pre>/);
});
