## 1. Foundation
- [ ] 1.1 Create Go module + minimal HTTP server skeleton
- [ ] 1.2 Add in-memory task store with lifecycle + scoring
- [ ] 1.3 Add worker runner (`exec` driver) with cancellation support
- [ ] 1.4 Add SSE event hub and wire task/log events
- [ ] 1.5 Add observer interface + MVP heuristic observer
- [ ] 1.6 Add API handlers: tasks, events, chat, system info

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
- [ ] 4.1 Unit tests for task store lifecycle + scoring
- [ ] 4.2 Unit tests for worker runner cancellation + log capture
- [ ] 4.3 HTTP tests for API endpoints (tasks + chat)
- [ ] 4.4 Smoke test script for one-command startup

