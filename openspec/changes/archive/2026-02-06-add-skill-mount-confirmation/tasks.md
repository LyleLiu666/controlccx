## 1. Implementation
- [x] 1.1 Add prompt skill token parser (codex/claude)
- [x] 1.2 Add “skill mount preflight” state in New Run flow
- [x] 1.3 Implement confirmation modal UI + narrow layout check
- [x] 1.4 Call `/api/skills/link` for selected target on confirm
- [x] 1.5 Normalize skill tokens for execution in backend worker (codex/claude)
- [x] 1.6 Add unit tests for parsing + preflight decisions
- [x] 1.7 Add Go unit tests for backend normalization

## 2. Verification
- [x] 2.1 Run `pnpm -C web test`
- [x] 2.2 Manual checklist:
  - [x] Codex run with `$missing-skill` prompts mount
  - [x] “Continue without mounting” starts run unchanged
  - [x] Prefix mismatch (`/skill` in codex) still works (backend normalization)
  - [x] Mobile/narrow width modal remains usable
- [x] 2.3 Run `go test ./...`
