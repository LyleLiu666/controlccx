# Design: ControlCCX Foundation

## High-level Architecture

### Components
- **Backend (Go)**: owns the source of truth for tasks, emits events, serves API + UI, and coordinates durable state.
- **Web UI (Vue 3 + Vite)**: consumes API + SSE to display tasks/logs and provides a chat interface.
- **Observer**: a backend component that answers user questions using:
  - task store (state + recent logs),
  - system info snapshot,
  - recent conversation.
- **Worker Executor**: runs the configured CLI tool (Claude Code / Codex) as an OS process, parses streaming JSON output, persists logs, and captures the session/thread ID for resume.

### Data Flow
1. UI calls `POST /api/tasks` to start a worker.
2. Backend creates task, spawns a worker process, and emits:
   - `task.created`, `task.updated`
   - `task.log` (stdout/stderr lines)
3. UI subscribes to `GET /api/events` (SSE) and updates in real time.
4. UI calls `POST /api/chat` with a user message.
5. Observer reads task store + system info and returns an assistant message.
6. Backend emits `chat.message` events so all connected clients see the response.

## Event Transport
Use **Server-Sent Events (SSE)** for v1:
- Simple to implement and deploy.
- Works well for “server -> browser” streams (task state/logs).
- Client -> server remains normal HTTP (`POST /api/...`).

If we later need bidirectional features (typing indicators, etc.), we can add WebSocket as a new change.

## Worker Model

### Worker Driver: `exec`
- Runs a command with args using `os/exec` (no shell).
- Captures stdout/stderr line-by-line.
- Cross-platform.

## Persistence & Resume (断点接续)

### Durable store
Use SQLite as the default embedded database for:
- tasks (metadata + status + scoring inputs),
- chat sessions/messages,
- logs (either in DB or log files, depending on implementation choice),
- worker session/thread identifiers required for resume.

To keep startup simple and cross-platform, prefer a pure-Go SQLite driver (no CGO).

### Resume strategy (session-based)
We follow the practical pattern used by existing CLI wrappers:
- Each worker run streams JSON output that includes a **session/thread ID**.
- The backend persists this ID with the task.
- “Resume” starts a new OS process using the CLI’s resume flag:
  - Claude Code: `-r <session_id>` (stream-json)
  - Codex: `resume <session_id>` (JSONL/JSON stream)

If the server is stopped while a task is running:
- the task is marked `interrupted` on next startup,
- the UI can offer a “resume” action that starts a new run against the persisted session ID.

## “Problematic task” Heuristic (v1)
We need a deterministic way to answer “哪个任务问题比较多”.
v1 scoring (transparent and testable):
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
