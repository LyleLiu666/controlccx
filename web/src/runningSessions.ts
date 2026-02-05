import type { Task, TaskStatus } from "./types.ts";

const IN_FLIGHT_STATUSES: ReadonlySet<TaskStatus> = new Set<TaskStatus>([
  "queued",
  "waiting",
  "running",
]);

export type RunningSessionCandidate = {
  key: string;
  run_id: string;
  status: TaskStatus;
  title: string;
  workdir: string;
  updated_at: string;
  in_flight_runs: number;
};

function sessionKeyForTask(t: Task): string {
  const cid = String(t?.conversation_id ?? "").trim();
  if (cid) return `c:${cid}`;
  const sid = String(t?.session_id ?? "").trim();
  if (sid) return `s:${sid}`;
  return `t:${String(t?.id ?? "").trim()}`;
}

function timeKeyForTask(t: Task): string {
  return String(
    t?.updated_at ??
      t?.started_at ??
      t?.created_at ??
      "",
  ).trim();
}

export function listRunningSessionCandidates(tasks: Task[]): RunningSessionCandidate[] {
  const list = Array.isArray(tasks) ? tasks : [];
  if (list.length === 0) return [];

  const groups = new Map<string, Task[]>();
  for (const t of list) {
    const key = sessionKeyForTask(t);
    if (!key) continue;
    const cur = groups.get(key) ?? [];
    cur.push(t);
    groups.set(key, cur);
  }

  const out: RunningSessionCandidate[] = [];
  for (const [key, runs] of groups.entries()) {
    const inFlight = runs.filter((r) => IN_FLIGHT_STATUSES.has(r.status));
    if (inFlight.length === 0) continue;

    let rep = inFlight[0]!;
    for (const r of inFlight.slice(1)) {
      if (timeKeyForTask(r) > timeKeyForTask(rep)) rep = r;
    }

    let title = "";
    for (const r of runs) {
      const s = String(r.session_title ?? "").trim();
      if (s) {
        title = s;
        break;
      }
    }

    out.push({
      key,
      run_id: String(rep.id ?? "").trim(),
      status: rep.status,
      title,
      workdir: String(rep.workdir ?? "").trim(),
      updated_at: String(rep.updated_at ?? "").trim(),
      in_flight_runs: inFlight.length,
    });
  }

  out.sort((a, b) => {
    const ta = String(a.updated_at ?? "").trim();
    const tb = String(b.updated_at ?? "").trim();
    if (ta && tb && ta !== tb) return tb.localeCompare(ta);
    return b.key.localeCompare(a.key);
  });
  return out;
}

