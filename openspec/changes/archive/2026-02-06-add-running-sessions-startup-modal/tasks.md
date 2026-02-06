## 1. Implementation
- [x] 1.1 Add pure helper to list “running sessions” from tasks
- [x] 1.2 Add startup modal component (click-overlay dismiss)
- [x] 1.3 Wire modal to show once per page load
- [x] 1.4 Add/adjust CSS for narrow layout
- [x] 1.5 Add frontend tests (helper + wiring smoke)

## 2. Verification
- [x] 2.1 Run `pnpm -C web test`
- [x] 2.2 Run `pnpm -C web build`
- [x] 2.3 Run `pnpm smoke`
- [x] 2.4 Manual checklist (narrow screen) (covered by `web/tests/runningSessionsStartupModalUi.test.ts`)
