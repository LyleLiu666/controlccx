# Project Context

## Purpose
ControlCCX is a local “control center” for running multiple agent workers (e.g., Claude Code and Codex) asynchronously,
while an observer assistant monitors execution using task/log/system context and supports real-time conversation.

## Tech Stack
- Backend: Go (HTTP API, SSE, worker orchestration)
- Frontend: Vue 3 + Vite (task dashboard + chat UI)
- Persistence: SQLite (embedded, cross-platform)

## Project Conventions

### Code Style
- Keep changes minimal and readable; avoid clever abstractions without need.
- Prefer explicit naming (no 1-letter variables except trivial loops).
- New packages should be small and focused.

### Architecture Patterns
- Backend owns the source of truth for tasks and persistence.
- Real-time updates use SSE for server-to-browser streaming.
- Worker execution is process-based; interactive CLIs run behind a runner that manages PTY/ConPTY and supports resume.
- Observer is pluggable (heuristic baseline + optional external LLM providers).

### Testing Strategy
- Prefer TDD: add/adjust tests first, then implement.
- Unit tests for core domain logic (task store, scoring, persistence).
- HTTP tests for API surface.
- Cross-compile checks for Windows build tags where runtime testing is unavailable.

### Iteration Loop (Required)
Every iteration in this repo MUST follow the same closed loop:

1) **Docs first**: write/update OpenSpec change docs (`proposal.md`, `tasks.md`, and delta `spec.md`).
2) **Order by foundations**: arrange development order based on dependencies (foundation → upper layers).
3) **Implement**: execute tasks sequentially.
4) **Test**: run the smallest relevant validations first, then broader checks.
5) **Fix**: iterate until validations pass.
6) **Docs again**: update documentation and mark completed tasks (`- [x]`) to reflect reality.

### Roadmap (Foundation → Upper Layers)
This ordering is driven by `docs/compare_webcode.md` (gap list + dependency reasoning). The goal is to build a “cockpit”
with strong foundations first, then add higher-level productivity layers.

0) **Foundation (baseline reliability)**
   - `add-controlccx-foundation` (done)

1) **Safety & Governance (must precede powerful capabilities)**
   - `add-approval-blocked-mvp` (done)
   - `add-claude-auto-approve-toggle` (done)
   - `add-delivery-foreman-check` (in progress)
   - `add-approval-trilogy` (planned; implement after docs/spec are finalized)

2) **Mobile Shell (interaction foundation)**
   - `add-mobile-shell` (done)

3) **Workspace File Ops (close the loop; allow safe writes)**
   - `add-workspace-file-ops` (done)

4) **Preview Tabs (Markdown/Raw/HTML preview)**
   - `add-preview-tabs` (done)

5) **Session Management (lower priority, but must exist)**
   - `add-session-management` (planned)

6) **Tooling Extensibility (multi-tool adapters + tool-level env)**
   - `add-tooling-extensibility` (planned)

7) **Foreman Dashboard (operational overview)**
   - `add-foreman-dashboard` (planned)

### Git Workflow
- Small, focused changes.
- Commit messages should describe the user-visible effect.

## Domain Context
- “Observer” answers user questions grounded in task state/log/system info (not hallucinated).
- “Worker” is an external tool runner (Claude Code, Codex, or generic exec) producing logs and requiring occasional input/approval.
- “断点接续” means task history is durable and in-flight work can be resumed/reattached when feasible.

## Important Constraints
- One-command start, cross-platform, simple local operation.
- Claude Code should behave consistently across OSes (Windows via Git Bash).
- Persistence and resume are required (industrial-grade usability).

## External Dependencies
- Optional: external LLM APIs for the observer (configured via environment/config).
- External CLI tools: `claude` and `codex` executables (configured paths or expected on `PATH`).
