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
