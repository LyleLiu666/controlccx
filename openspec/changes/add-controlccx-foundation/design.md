# Design: ControlCCX Foundation

## High-level Architecture

### Components
- **Backend (Go)**: owns the source of truth for tasks/workers, emits events, serves API + UI, and coordinates durable state.
- **Web UI (Vue 3 + Vite)**: consumes API + SSE to display tasks/logs and provides a chat interface.
- **Observer**: a backend component that answers user questions using:
  - task store (state + recent logs),
  - system info snapshot,
  - recent conversation.
- **Worker Runner**: a long-lived per-task process that hosts the interactive session (PTY) and persists logs/state so tasks can be resumed after backend restart.

### Data Flow
1. UI calls `POST /api/tasks` to start a worker.
2. Backend creates task, spawns a worker runner, and emits:
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

### Worker Driver: `pty` (interactive CLIs)
To wrap interactive CLIs (Claude Code / Codex) safely, we use a PTY-backed driver to:
- detect prompts (e.g. permission confirmations),
- pause and request observer decision,
- write the chosen response back to the PTY,
- preserve interactive behavior consistently across platforms (Claude Code especially).

Implementation notes:
- **Linux/macOS**: use a Unix PTY (e.g. `creack/pty`) with efficient buffered IO.
- **Windows**: use ConPTY for PTY support. For **Claude Code**, run via Git Bash to keep behavior closer to *nix; for **Codex**, support is best-effort due to PowerShell variance.

## Persistence & Resume (断点接续)

### Durable store
Use SQLite as the default embedded database for:
- tasks (metadata + status + scoring inputs),
- chat sessions/messages,
- worker runner metadata (runner PID, control address, created/updated timestamps),
- log indexes/counters (with log payload stored separately).

To keep startup simple and cross-platform, prefer a pure-Go SQLite driver (no CGO).

### Log storage
Store full logs as append-only files per task (bounded rotation optional), while keeping counters + last offsets in the DB:
- avoids DB bloat for long-running sessions,
- enables fast “problem score” computation without re-parsing the entire log.

### Resume strategy
Each task is backed by a **worker runner process** that:
- hosts the PTY and the child CLI process,
- writes logs to file and updates DB counters/status,
- exposes a local control channel (Unix socket / Windows named pipe) for:
  - sending input,
  - canceling,
  - reading missed output on reconnect.

On backend restart:
- backend reloads tasks from DB,
- re-connects to any live runners via the stored control address,
- resumes event streaming and UI interaction without losing task history.

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
