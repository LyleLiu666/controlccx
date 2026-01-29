## 1. OpenSpec
- [x] 1.1 Add delta specs (`observer-assistant`, `web-ui`)
- [x] 1.2 Run `openspec validate add-attention-autopilot --strict --no-interactive`

## 2. Implementation
- [x] 2.1 Add an “Attention Autopilot” toggle (persisted) in Secretary Overview
- [x] 2.2 Auto-resume interrupted sessions once (rate-limited + deduped)
- [x] 2.3 Surface clear autopilot outcome messages (success/failure/why stopped)

## 3. Verification
- [x] 3.1 Add unit tests for dedupe/rate-limit logic (or the smallest adjacent testable module)
- [x] 3.2 Run `go test ./...`
- [x] 3.3 Run `npm -C web run build` (or the repo’s equivalent frontend build)
