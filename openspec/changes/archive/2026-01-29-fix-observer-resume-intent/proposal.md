# Change: fix-observer-resume-intent

## Why
The observer assistant can interpret user messages as “resume the interrupted session” and automatically start a resume run.
Today, overly-broad keyword matching can trigger resume unintentionally when the user merely *mentions* words like “continue”.

This creates surprising behavior and can start meaningless runs, which violates the cockpit principle of predictable governance.

## What Changes
- Tighten resume intent detection so it only triggers on **explicit commands** (e.g. “继续”, “resume”, “retry”, “continue” as a
  command), not on casual mentions in longer sentences.
- Keep explicit prefix targeting (resume by run-id/session-id prefix) working as before.

## Principles & Metrics
- Improves **Resumability and resilience** (resume only when user intends it).
- Improves **Safety by default, with explicit governance** (avoid unintended automation).

## Impact
- Backend observer heuristics:
  - `internal/observer/observer.go`
- Tests:
  - `internal/observer/observer_test.go`

