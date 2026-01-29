## 1. Update OpenSpec Change Docs (this change)
- [x] Ensure delta specs exist under `openspec/changes/update-codex-sandbox-mode/specs/` with scenarios
- [x] Run `openspec validate update-codex-sandbox-mode --type change --strict --no-interactive`

## 2. Implementation
- [x] Update Codex tool invocation to default to `--sandbox workspace-write` (safe mode only)
- [x] Add/adjust unit tests to cover the new sandbox flag and preserve unsafe behavior

## 3. Verification
- [x] Run `go test ./...`

## 4. Close the Loop
- [x] Update `openspec/project.md` timeline (prepend newest)
- [x] Archive the change: `openspec archive update-codex-sandbox-mode --skip-specs --yes`
