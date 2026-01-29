# Change: update-codex-sandbox-mode

## Why
When running the Codex worker non-interactively, Codex may treat the environment as read-only and refuse to modify files.
This is surprising and makes “Codex as an agent” ineffective for tasks that are expected to produce real workspace changes.

We already have an explicit safety governance model (approval workflow + an explicit unsafe mode), so we should keep “safe by
default” while allowing typical workspace write behavior.

## What Changes
- Run Codex in **workspace-write** sandbox mode by default (safe mode) so it can edit files in the workspace.
- Preserve the existing explicit unsafe path (`--dangerously-bypass-approvals-and-sandbox`) for cases where the user opts into
  fully bypassing approvals/sandboxing.

## Principles & Metrics
- Improves **Safety by default, with explicit governance** (workspace write is sandboxed; unsafe mode is explicit).
- Improves **Time-to-start** and **Continuity** indirectly by avoiding “it ran but couldn’t do anything” outcomes.

## Impact
- Backend worker command builder for Codex:
  - `internal/worker/tools.go`
- Tests:
  - `internal/worker/tools_unsafe_flags_test.go`

