## 1. Foundation
- [x] 1.1 Create Go module + minimal HTTP server skeleton
- [x] 1.2 Add SQLite persistence (tasks, logs, chat) + migrations (WAL enabled)
- [x] 1.3 Add task store on DB with lifecycle + scoring inputs
- [x] 1.4 Implement CLI tool adapters (claude-code, codex) with path-based config
- [x] 1.5 Implement streaming JSON parser and persist logs + session/thread IDs
- [x] 1.6 Implement task executor with cancellation + resume (session-based)
- [x] 1.7 Add SSE event hub and wire task/log/chat events
- [x] 1.8 Add observer interface + heuristic observer (grounded answers)
- [x] 1.9 Add API handlers: tasks, events, chat, system info, logs

## 2. Web UI
- [x] 2.1 Create Vue 3 + Vite app scaffold
- [x] 2.2 Implement API client + SSE subscription
- [x] 2.3 Implement task dashboard (list/detail/log stream)
- [x] 2.4 Implement chat UI (user + observer messages)
- [x] 2.5 Implement system info panel

## 3. One-command Start
- [x] 3.1 Add root `package.json` scripts: `dev`, `build`, `start`
- [x] 3.2 Configure Vite proxy for `/api/*` and `/api/events`
- [x] 3.3 Add production build pipeline (embed built web assets into server binary)

## 4. Validation (TDD)
- [x] 4.1 Unit tests for DB-backed task store lifecycle + scoring
- [x] 4.2 Unit tests for streaming parser + session/thread ID extraction
- [x] 4.3 Unit tests for log persistence + counters
- [x] 4.4 Unit tests for resume flow (interrupted -> resumable)
- [x] 4.5 HTTP tests for API endpoints (tasks + logs + chat)
- [x] 4.6 Cross-compile check for Windows (build tags + `GOOS=windows`)
- [x] 4.7 Smoke test script for one-command startup
