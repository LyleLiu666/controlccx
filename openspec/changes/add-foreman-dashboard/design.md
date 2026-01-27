## Context
ControlCCX orchestrates external CLI workers (Claude Code, Codex, exec) and persists task history/logs for resume.
The UI needs to scale from “single run” to “many concurrent sessions” while preserving traceability and confidence.

## Goals / Non-Goals
- Goals:
  - Make **sessions** the primary UI unit (“subordinates”)
  - Make **workspaces** first-class filters and shortcuts
  - Improve **traceability**: see command/args/dir, log search/export, replay/resume
  - Keep secret values safe (show env key names only, never values)
- Non-Goals:
  - Autonomous agent approvals/actions without explicit user confirmation
  - Multi-host scheduling
  - Introducing a full plugin framework

## Decisions
- Decision: Persist structured run invocation metadata.
  - Why: Text-only logs are hard to query and brittle to parse; a trace view should be reliable and searchable.
  - Shape (tentative):
    - New table `task_invocations` (or extend tasks) storing:
      - `task_id`, `worker_type`, `dir`, `command`, `args_json`, `env_injected_keys_json`, `started_at`
    - Also keep human-readable `system` log lines for quick scanning and backward compatibility.
- Decision: Add server-side log query primitives.
  - Why: Client-side filtering becomes slow/incomplete with long-running sessions.
  - Approach: Extend log list API with optional filters (streams, query substring, time/id ranges) and an export route.
- Decision: “Replay” is a clone operation, not a byte-perfect process re-run.
  - Why: CLIs evolve and local tool paths/config differ; the stable re-run inputs are:
    - worker type, workdir, prompt, and (optionally) session_id for resume mode.
  - The trace view still documents the exact invocation used at that time.

## Risks / Trade-offs
- Schema migration complexity (SQLite):
  - Mitigation: Keep migration additive, small, and covered by tests; fallback to reading from `system` logs if needed.
- Leaking secrets via trace/log export:
  - Mitigation: Persist only env key names; redact any suspicious values in export formatting.

## Migration Plan
1. Add schema migration for invocation records (additive).
2. Write backfill (optional) for recent tasks by parsing existing `run.start` system logs.
3. Update APIs + UI to prefer structured invocation data; fall back to parsed logs if missing.

## Open Questions
- Should “workspace” be a saved catalog entity (name + path), or remain “path-only” with pinning?
- Should log search be substring-only (fast) or regex (powerful but riskier)?
- What is the minimal replay UX that feels safe (confirm dialog, diff view of parameters, etc.)?

