## 1. Observer (Backend)
- [x] 1.1 Add fallback intent detection for “继续/恢复/重试” and invoke existing resume flow
- [x] 1.2 Deterministically pick a target task when user doesn’t specify one
- [x] 1.3 Ensure the reply clearly states what was resumed (new run id/session/workdir)

## 2. Tests
- [x] 2.1 Unit test: “继续” resumes the most recent interrupted/failed session (no LLM backend)
- [x] 2.2 Unit test: explicit id/session reference resumes that target

## 3. Verification
- [x] 3.1 Run `go test ./...`
- [x] 3.2 Run `openspec validate update-secretary-autoresume --strict --no-interactive`
