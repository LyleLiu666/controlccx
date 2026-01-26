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
  - A pluggable observer interface (MVP: heuristic observer; later: real LLM).
-  - Durable persistence and resume (tasks/logs survive restart; long-running tasks can be re-attached).
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
- Worker execution is local process-based, supporting both non-interactive exec and interactive PTY-backed CLIs.
- Task persistence uses an embedded database (SQLite) by default for cross-platform portability.

## Open Questions (need confirmation before implementation)
1. **DB location**: default data dir should be:
   - A) `~/.controlccx/` (recommended), or
   - B) project-local `./.controlccx/` (easier per-repo isolation)?
2. **Claude Code binary**: should we assume `claude` is on `PATH`, or add explicit config for:
   - the `claude` executable path and
   - Git Bash `bash.exe` path on Windows?
3. **Resume semantics** (“断点接续”): for tasks that were `running` at crash/restart, should we:
   - A) guarantee the worker process keeps running (detached runner) and reattach, or
   - B) mark as `orphaned` and offer an explicit “restart task” action (safer if reattach is impossible)?

## Acceptance Criteria
- A user can start the app with a single command and open a browser to:
  - see tasks running in real time,
  - see logs streaming,
  - ask the observer about current tasks and get a grounded answer.
- The backend builds on macOS/Linux/Windows and the dev workflow is cross-platform.
- Task data survives restart and can be reloaded; “in-flight” tasks support resume/reattach per the chosen semantics.
- The API surface is fully described by OpenSpec deltas in this change.
