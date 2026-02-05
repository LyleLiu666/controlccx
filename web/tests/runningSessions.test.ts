import assert from "node:assert/strict";
import test from "node:test";

import { listRunningSessionCandidates } from "../src/runningSessions.ts";

function mkTask(partial: any) {
  return {
    id: partial.id ?? "t1",
    conversation_id: partial.conversation_id ?? "",
    worker_type: partial.worker_type ?? "codex",
    mode: partial.mode ?? "new",
    status: partial.status ?? "succeeded",
    prompt: partial.prompt ?? "",
    workdir: partial.workdir ?? "/tmp",
    session_id: partial.session_id ?? "",
    session_title: partial.session_title ?? "",
    warning: partial.warning ?? "",
    error: partial.error ?? "",
    stderr_count: partial.stderr_count ?? 0,
    keyword_count: partial.keyword_count ?? 0,
    score: partial.score ?? 0,
    created_at: partial.created_at ?? "2026-02-05T10:00:00Z",
    updated_at: partial.updated_at ?? "2026-02-05T10:00:00Z",
    started_at: partial.started_at,
    finished_at: partial.finished_at,
  } as any;
}

test("listRunningSessionCandidates returns empty when no in-flight runs", () => {
  const out = listRunningSessionCandidates([
    mkTask({ id: "a", status: "succeeded" }),
    mkTask({ id: "b", status: "failed" }),
  ]);
  assert.deepEqual(out, []);
});

test("listRunningSessionCandidates groups by conversation_id (preferred) and picks newest in-flight run", () => {
  const out = listRunningSessionCandidates([
    mkTask({
      id: "old",
      conversation_id: "c1",
      session_id: "s1",
      status: "running",
      updated_at: "2026-02-05T10:00:00Z",
    }),
    mkTask({
      id: "new",
      conversation_id: "c1",
      session_id: "s1",
      status: "queued",
      updated_at: "2026-02-05T10:05:00Z",
    }),
  ]);
  assert.equal(out.length, 1);
  assert.equal(out[0]?.key, "c:c1");
  assert.equal(out[0]?.run_id, "new");
  assert.equal(out[0]?.status, "queued");
});

test("listRunningSessionCandidates falls back to session_id when conversation_id is missing", () => {
  const out = listRunningSessionCandidates([
    mkTask({ id: "t1", session_id: "s1", status: "waiting" }),
  ]);
  assert.equal(out.length, 1);
  assert.equal(out[0]?.key, "s:s1");
});

test("listRunningSessionCandidates sorts sessions by updated_at desc", () => {
  const out = listRunningSessionCandidates([
    mkTask({ id: "a", conversation_id: "c1", status: "running", updated_at: "2026-02-05T10:00:00Z" }),
    mkTask({ id: "b", conversation_id: "c2", status: "running", updated_at: "2026-02-05T10:10:00Z" }),
  ]);
  assert.deepEqual(out.map((x) => x.key), ["c:c2", "c:c1"]);
});

