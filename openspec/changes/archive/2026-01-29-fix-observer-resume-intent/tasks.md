## 1. Update OpenSpec Change Docs (this change)
- [x] Ensure delta specs exist under `openspec/changes/fix-observer-resume-intent/specs/` with scenarios
- [x] Run `openspec validate fix-observer-resume-intent --type change --strict --no-interactive`

## 2. Implementation
- [x] Make resume intent detection conservative (explicit commands only)
- [x] Add regression tests so “mentioning continue” does not start a resume run

## 3. Verification
- [x] Run `go test ./...`

## 4. Close the Loop
- [x] Update `openspec/project.md` timeline (prepend newest)
- [x] Archive: `openspec archive fix-observer-resume-intent --skip-specs --yes`
