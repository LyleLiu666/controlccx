# TEMP: Secretary Rev2 Implementation Plan (8 Commits)

Date: 2026-03-18
Mode: strict checkpoint commits (one node = one commit)

## Node Checklist

- [ ] 1) plan: freeze this temporary execution plan document
- [ ] 2) db: chat_messages add conversation_id + backfill + index (+ migration tests)
- [ ] 3) api: /secretary/messages + stream support conversation_id pass-through (+ API tests)
- [ ] 4) api: /secretary/clear define partition semantics (+ API tests)
- [ ] 5) service: secretary history/send/clear partitioned by conversation (+ service/chat tests)
- [ ] 6) scheduler: bind schedule jobs to conversation context (+ scheduler tests)
- [ ] 7) guard: write-capable action_plan guard with fail-closed/fail-open semantics (+ tool tests)
- [ ] 8) ops: feature flags + observability/audit fields wiring (+ config/tests)

## Execution Rules

1. TDD first: write or adjust tests before behavior changes.
2. Every node ends with:
   - tests green for touched scope
   - one commit only for that node
3. No commit mixing between nodes.
4. If blocked, record blocker and stop before next node.
