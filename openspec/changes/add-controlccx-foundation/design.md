# Design: ControlCCX Foundation

## High-level Architecture

### Components
- **Backend (Go)**: owns the source of truth for tasks/workers, runs workers, emits events, and serves API + UI.
- **Web UI (Vue 3 + Vite)**: consumes API + SSE to display tasks/logs and provides a chat interface.
- **Observer**: a backend component that answers user questions using:
  - task store (state + recent logs),
  - system info snapshot,
  - recent conversation.

### Data Flow
1. UI calls `POST /api/tasks` to start a worker.
2. Backend creates task, runs worker asynchronously, and emits:
   - `task.created`, `task.updated`
   - `task.log` (stdout/stderr lines)
3. UI subscribes to `GET /api/events` (SSE) and updates in real time.
4. UI calls `POST /api/chat` with a user message.
5. Observer reads task store + system info and returns an assistant message.
6. Backend emits `chat.message` events so all connected clients see the response.

## Event Transport
Use **Server-Sent Events (SSE)** for MVP:
- Simple to implement and deploy.
- Works well for “server -> browser” streams (task state/logs).
- Client -> server remains normal HTTP (`POST /api/...`).

If we later need bidirectional features (typing indicators, etc.), we can add WebSocket as a new change.

## Worker Model

### MVP Worker Driver: `exec`
- Runs a command with args using `os/exec` (no shell).
- Captures stdout/stderr line-by-line.
- Cross-platform.

### Optional Driver: `pty` (future or limited scope)
To wrap interactive CLIs (Claude Code / Codex) safely, we likely need a PTY driver to:
- detect prompts (e.g. permission confirmations),
- pause and request observer decision,
- write the chosen response back to the PTY.

Windows support for PTY differs (ConPTY); this may be deferred.

## “Problematic task” Heuristic (MVP)
We need a deterministic way to answer “哪个任务问题比较多”.
MVP scoring (transparent and testable):
- +3 for `blocked` status (waiting for approval / prompt).
- +2 per stderr line (capped).
- +2 for non-zero exit.
- +1 per “error|panic|failed” keyword in logs (capped).

Observer can use this scoring to rank tasks without depending on an external LLM.

## One-command Startup
Cross-platform startup is easiest using `pnpm` scripts:
- `pnpm dev` uses a JS process runner (e.g. `concurrently`) to start:
  - Go backend (hot reload optional),
  - Vite dev server with proxy to backend.
- `pnpm start` builds the web assets and runs the Go server serving the built UI.

This keeps the user experience “one command”, even though internally it runs two processes in dev.

