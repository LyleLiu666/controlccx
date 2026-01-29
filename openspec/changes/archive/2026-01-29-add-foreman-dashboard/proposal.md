# Proposal: Foreman Dashboard (Sessions, Workspaces, Traceability)

## Why
ControlCCX is not a chat bot UI; it is a “foreman console” to direct multiple worker sessions in parallel.
When a user manages ~10+ concurrent sessions across different folders (workspaces), they need:

- Fast session switching and workspace scoping
- A global “secretary” overview (not per-session) that highlights what needs attention
- Execution traceability: see what actually ran, search logs, export, and replay/resume with confidence

## What Changes
- UI focuses on **sessions as subordinates**:
  - Sessions list as the primary navigation object (grouped by `session_id`)
  - Each session shows a run history and supports quick switching between runs
- UI supports **workspace-first filtering**:
  - Workspace selector with pinning, recent entries, and optional display names for pinned workspaces
  - One-click “focus workdir” actions from session/run views
- Add **execution traceability** features:
  - Record and display run invocation metadata (cmd/args/dir + injected env key names only)
  - Log filtering (stream toggles), substring search, and export (download)
  - “Replay run” and “Resume session” shortcuts grounded in persisted session IDs
- Expand the global **Secretary**:
  - Cross-session attention queue (blocked/failed/high-score)
  - Briefing summary for the current workspace or all sessions

## Impact
- Affected code:
  - Frontend: session dashboard UX, log/trace panels
  - Backend: task run metadata persistence, log query/export endpoints
  - DB: may require a small schema migration for structured run invocation records
- Affected specs:
  - `control-sessions`
  - `trace-execution`

## Out of Scope (this change)
- Multi-user authn/authz, RBAC
- Fully autonomous decision making (secretary actions remain user-confirmed)
- Distributed workers across multiple hosts

## Acceptance Criteria
- Users can manage 10+ sessions across multiple workspaces without losing context.
- For any run, users can answer “what exactly ran?” via a first-class trace view.
- Users can filter/search/export logs and can replay/resume with a single action.
- Secretary provides a trustworthy global view and quick navigation to “needs attention”.
