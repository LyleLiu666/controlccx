## 1. Foundation
- [ ] 1.1 Create Go module + minimal HTTP server skeleton
- [ ] 1.2 Add SQLite persistence (tasks, runners, chat) + migrations (WAL enabled)
- [ ] 1.3 Add task store on DB with lifecycle + scoring inputs
- [ ] 1.4 Add worker runner supervisor + control channel
- [ ] 1.5 Implement worker drivers:
  - `exec` (cross-platform)
  - `pty` on Linux/macOS
  - `conpty` on Windows (Claude Code via Git Bash)
- [ ] 1.6 Add log persistence (per-task log files + DB counters/offsets)
- [ ] 1.7 Add SSE event hub and wire task/log/chat events
- [ ] 1.8 Add observer interface + heuristic observer (grounded answers)
- [ ] 1.9 Add API handlers: tasks, events, chat, system info, logs

## 2. Web UI
- [ ] 2.1 Create Vue 3 + Vite app scaffold
- [ ] 2.2 Implement API client + SSE subscription
- [ ] 2.3 Implement task dashboard (list/detail/log stream)
- [ ] 2.4 Implement chat UI (user + observer messages)
- [ ] 2.5 Implement system info panel

## 3. One-command Start
- [ ] 3.1 Add root `package.json` scripts: `dev`, `build`, `start`
- [ ] 3.2 Configure Vite proxy for `/api/*` and `/api/events`
- [ ] 3.3 Add production build pipeline (Go serves `web/dist`)

## 4. Validation (TDD)
- [ ] 4.1 Unit tests for DB-backed task store lifecycle + scoring
- [ ] 4.2 Unit tests for log persistence + counters
- [ ] 4.3 Unit tests for runner supervisor (resume/reattach semantics)
- [ ] 4.4 HTTP tests for API endpoints (tasks + logs + chat)
- [ ] 4.5 Cross-compile check for Windows (build tags + `GOOS=windows`)
- [ ] 4.6 Smoke test script for one-command startup
