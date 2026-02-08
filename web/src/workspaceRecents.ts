import type { Task } from "./types.ts";

export const MAX_WORKSPACE_RECENTS = 20;

function clampMax(max: number): number {
  if (!Number.isFinite(max)) return MAX_WORKSPACE_RECENTS;
  return Math.max(0, Math.trunc(max));
}

function normalizePathForCompare(p: string): string {
  let s = p.trim();
  if (!s) return "";
  s = s.replaceAll("\\", "/").replace(/\/+/g, "/");
  while (s.length > 1 && s.endsWith("/")) s = s.slice(0, -1);
  if (/^[a-zA-Z]:/.test(s)) s = s.toLowerCase();
  return s;
}

function workspacePathKey(path: string): string {
  return normalizePathForCompare(path);
}

function dedupeWorkspaceList(paths: string[], max: number): string[] {
  const limit = clampMax(max);
  if (limit === 0) return [];

  const out: string[] = [];
  const seen = new Set<string>();
  for (const raw of paths) {
    const path = String(raw ?? "").trim();
    if (!path) continue;
    const key = workspacePathKey(path);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push(path);
    if (out.length >= limit) break;
  }
  return out;
}

function taskTimeKey(t: Task): string {
  return String(t?.created_at ?? "").trim();
}

export function listRecentWorkspacePaths(tasks: Task[], max = MAX_WORKSPACE_RECENTS): string[] {
  const limit = clampMax(max);
  if (limit === 0) return [];

  const latestByPath = new Map<string, { path: string; time: string }>();
  for (const t of Array.isArray(tasks) ? tasks : []) {
    const path = String(t?.workdir ?? "").trim();
    if (!path) continue;
    const key = workspacePathKey(path);
    if (!key) continue;
    const time = taskTimeKey(t);

    const prev = latestByPath.get(key);
    if (!prev || time > prev.time) {
      latestByPath.set(key, { path, time });
    }
  }

  return Array.from(latestByPath.values())
    .sort((a, b) => {
      if (a.time && b.time && a.time !== b.time) return b.time.localeCompare(a.time);
      return b.path.localeCompare(a.path);
    })
    .map((x) => x.path)
    .slice(0, limit);
}

export function rememberWorkspacePath(
  recents: string[],
  path: string,
  max = MAX_WORKSPACE_RECENTS,
): string[] {
  const list = Array.isArray(recents) ? recents : [];
  return dedupeWorkspaceList([path, ...list], max);
}

export function mergeWorkspaceRecents(
  primary: string[],
  secondary: string[],
  max = MAX_WORKSPACE_RECENTS,
): string[] {
  const a = Array.isArray(primary) ? primary : [];
  const b = Array.isArray(secondary) ? secondary : [];
  return dedupeWorkspaceList([...a, ...b], max);
}
