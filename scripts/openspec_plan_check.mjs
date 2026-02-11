#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

export function loadManifest(manifestPath) {
  const raw = fs.readFileSync(manifestPath, "utf8");
  const data = JSON.parse(raw);
  if (!data || typeof data !== "object") throw new Error("invalid manifest object");
  if (!Array.isArray(data.changes) || data.changes.length === 0) throw new Error("manifest.changes is empty");
  return data;
}

export function findChangeDir(repoRoot, changeID) {
  const activeDir = path.join(repoRoot, "openspec", "changes", changeID);
  if (fs.existsSync(activeDir)) return { dir: activeDir, archived: false };

  const archiveRoot = path.join(repoRoot, "openspec", "changes", "archive");
  if (!fs.existsSync(archiveRoot)) return null;
  const suffix = `-${changeID}`;
  const candidates = fs
    .readdirSync(archiveRoot, { withFileTypes: true })
    .filter((ent) => ent.isDirectory() && ent.name.endsWith(suffix))
    .map((ent) => path.join(archiveRoot, ent.name))
    .sort();
  if (candidates.length === 0) return null;
  return { dir: candidates[candidates.length - 1], archived: true };
}

export function isChangeCompleted(tasksPath) {
  if (!fs.existsSync(tasksPath)) return false;
  const raw = fs.readFileSync(tasksPath, "utf8");
  const items = raw.match(/^- \[[ xX]\] .+$/gm) || [];
  if (items.length === 0) return false;
  return items.every((line) => /^- \[[xX]\] /.test(line));
}

export function validateManifest(manifest, repoRoot) {
  const errors = [];
  const byID = new Map();
  for (const c of manifest.changes) {
    const id = String(c.id || "").trim();
    if (!id) {
      errors.push("change with empty id");
      continue;
    }
    if (byID.has(id)) {
      errors.push(`duplicate change id: ${id}`);
      continue;
    }
    byID.set(id, c);
  }

  const layers = Array.isArray(manifest.layers) ? manifest.layers.map((x) => String(x).trim()) : [];
  const layerIndex = new Map(layers.map((l, i) => [l, i]));

  for (const c of manifest.changes) {
    const id = String(c.id || "").trim();
    if (!id) continue;
    const layer = String(c.layer || "").trim();
    if (!layerIndex.has(layer)) {
      errors.push(`change ${id}: unknown layer ${layer}`);
    }
    const deps = Array.isArray(c.hard_dependencies) ? c.hard_dependencies : [];
    for (const dep of deps) {
      const d = String(dep || "").trim();
      if (!d) continue;
      if (!byID.has(d)) {
        errors.push(`change ${id}: missing dependency ${d}`);
        continue;
      }
      const depLayer = String((byID.get(d) || {}).layer || "").trim();
      if (layerIndex.has(depLayer) && layerIndex.has(layer) && layerIndex.get(depLayer) > layerIndex.get(layer)) {
        errors.push(`change ${id}: dependency ${d} is in later layer (${depLayer} > ${layer})`);
      }
    }

    const found = findChangeDir(repoRoot, id);
    if (!found) {
      errors.push(`change ${id}: missing change directory (active or archived)`);
      continue;
    }
    const dir = found.dir;
    const proposal = path.join(dir, "proposal.md");
    const tasks = path.join(dir, "tasks.md");
    if (!fs.existsSync(proposal)) errors.push(`change ${id}: missing proposal.md`);
    if (!fs.existsSync(tasks)) errors.push(`change ${id}: missing tasks.md`);
  }

  // Cycle detection
  const visiting = new Set();
  const visited = new Set();
  function dfs(id, stack) {
    if (visiting.has(id)) {
      errors.push(`cycle detected: ${[...stack, id].join(" -> ")}`);
      return;
    }
    if (visited.has(id)) return;
    visiting.add(id);
    const c = byID.get(id);
    const deps = Array.isArray(c?.hard_dependencies) ? c.hard_dependencies.map((x) => String(x || "").trim()).filter(Boolean) : [];
    for (const d of deps) {
      if (byID.has(d)) dfs(d, [...stack, id]);
    }
    visiting.delete(id);
    visited.add(id);
  }
  for (const id of byID.keys()) dfs(id, []);

  return {
    ok: errors.length === 0,
    errors,
    count: byID.size,
  };
}

export function computeReadySet(manifest, repoRoot) {
  const byID = new Map();
  for (const c of manifest.changes || []) {
    const id = String(c.id || "").trim();
    if (id) byID.set(id, c);
  }

  const completed = new Set();
  for (const id of byID.keys()) {
    const found = findChangeDir(repoRoot, id);
    if (!found) continue;
    if (found.archived) {
      completed.add(id);
      continue;
    }
    const tasksPath = path.join(found.dir, "tasks.md");
    if (isChangeCompleted(tasksPath)) completed.add(id);
  }

  const ready = [];
  for (const [id, c] of byID.entries()) {
    if (completed.has(id)) continue;
    const deps = Array.isArray(c.hard_dependencies) ? c.hard_dependencies.map((x) => String(x || "").trim()).filter(Boolean) : [];
    const blockedBy = deps.filter((d) => !completed.has(d));
    if (blockedBy.length === 0) ready.push({ id, layer: String(c.layer || "").trim(), blockedBy: [] });
  }
  ready.sort((a, b) => a.layer.localeCompare(b.layer) || a.id.localeCompare(b.id));
  return { ready, completed: [...completed].sort() };
}

function main() {
  const repoRoot = process.cwd();
  const manifestPath = path.join(repoRoot, "docs", "openspec", "plan-30-changes.manifest.json");
  const manifest = loadManifest(manifestPath);
  const out = validateManifest(manifest, repoRoot);

  if (!out.ok) {
    for (const e of out.errors) console.error(`ERROR: ${e}`);
    process.exit(1);
  }
  if (process.argv.includes("--ready")) {
    const rs = computeReadySet(manifest, repoRoot);
    console.log(`OK: validated ${out.count} changes in dependency plan`);
    console.log(`Completed: ${rs.completed.length}`);
    for (const id of rs.completed) console.log(`  - ${id}`);
    console.log(`Ready now: ${rs.ready.length}`);
    for (const it of rs.ready) console.log(`  - [${it.layer}] ${it.id}`);
    return;
  }
  console.log(`OK: validated ${out.count} changes in dependency plan`);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
