# Proposal: ControlCCX Foundation (Go + Vue)

## Why
We need a lightweight “control center” that can run multiple agent workers asynchronously (e.g., Claude Code / Codex),
while an “observer LLM” keeps watching execution, collecting signals (task state, logs, system info), and answering
real-time user questions like:

- “我们有几个任务在执行？”
- “哪个任务你觉得问题比较多？”

The project must be easy to start (one command), cross-platform, and simple to operate.

## What Changes
- Add a Go backend providing:
  - Task/worker lifecycle management (create/list/get/cancel).
  - Real-time event streaming (SSE) for task state/logs and observer responses.
  - System info endpoint for the observer and UI.
  - A pluggable observer interface (built-in heuristic + optional external LLM provider).
  - Durable persistence and resume (tasks/logs survive restart; long-running tasks can be re-attached).
- Add a Vue (Vite) web UI providing:
  - Task dashboard with live status/logs.
  - Chat panel to talk to the observer.
  - System info panel.
- Provide one-command startup:
  - `pnpm dev` runs backend + frontend for development.
  - `pnpm start` runs a production-like server serving the built UI.

## Scope (industrial-grade v1)
- **Workers**: MUST support both `claude code` and `codex` workers; Linux-first for stability/performance.
- **Cross-platform**:
  - `claude code` MUST have consistent behavior across macOS/Linux/Windows (Windows runs via Git Bash).
  - `codex` on Windows is supported but can be “best-effort” due to PowerShell instability.
- **Persistence & resume**: MUST persist tasks and logs, and support “断点接续” (restart does not lose task history; and in-flight work can be resumed/reattached when feasible).

## Out of scope (v1)
- Authn/authz (assume local-only deployment).
- Multi-host worker scheduling (single machine).
- Cloud deployment hardening (TLS, RBAC, etc.).

## Assumptions
- Users run this locally on a developer machine; scale is “developer-grade” but reliability is production-minded.
- Worker execution is local process-based, using each CLI’s supported streaming JSON output and session resume features.
- Task persistence uses an embedded database (SQLite) by default for cross-platform portability.

## Decisions (confirmed)
1. **Data dir**: `~/.controlccx/`
2. **Tool paths**: configured paths are supported (and preferred); fallback to `PATH` when not set.
3. **Resume semantics**: follow the pattern of session resume used by projects like `myclaude` / `WebCode`:
   - tasks and logs are persisted,
   - if a run is interrupted, the user can “resume” by starting a new run against the persisted session/thread ID.

## Acceptance Criteria
- A user can start the app with a single command and open a browser to:
  - see tasks running in real time,
  - see logs streaming,
  - ask the observer about current tasks and get a grounded answer.
- The backend builds on macOS/Linux/Windows and the dev workflow is cross-platform.
- Task data survives restart and can be reloaded; interrupted work can be resumed using persisted session/thread IDs.
- The API surface is fully described by OpenSpec deltas in this change.
