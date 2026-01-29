# Project Context

## Purpose
ControlCCX is a local “control center” for running multiple agent workers (e.g., Claude Code and Codex) asynchronously,
while an observer assistant monitors execution using task/log/system context and supports real-time conversation.

## Vision
一个以 Claude Code 和 Codex 为基础的 agent 系统：用户可以方便地创建任务、同时管理多个任务，免去 TUI 的不方便；易用、好用、安全、有流程；既能以简洁的 UX 满足普通用户，也提供足够的功能纵深满足高端用户。

## Product Principles (3–5)
These principles are used to order roadmap work, shape specs, and define “better” in reviews.

1) **Cockpit-first UX (progressive disclosure)**
   - Default UI MUST feel like a “驾驶室”：focus on the few high-frequency actions (create / monitor / resume / copy results).
   - Advanced controls MUST exist, but SHOULD be hidden behind progressive disclosure (menus/drawers) and never dominate the main canvas.

2) **Resumability and resilience**
   - Users MUST be able to resume/reattach to in-flight work without losing context.
   - If resume is impossible (e.g. the conversation/session truly does not exist), the system MUST fail fast with a clear, actionable error (not silently creating a meaningless new run).

3) **Safety by default, with explicit governance**
   - Powerful actions (workspace writes, deletions, risky commands) MUST be governed by an approval workflow.
   - The workflow MUST be predictable and explainable: users should understand “why it was blocked” and “what to do next”.

4) **Never feel stuck (observable progress)**
   - While work is running, the UI MUST always show what is happening “now” (active step / active tool / latest logs).
   - Long-running silence SHOULD be mitigated with explicit “still running” signals (heartbeats, tool activity, milestones).

5) **Local-first, cross-platform, one-command start**
   - The system SHOULD run locally, be cross-platform, and be easy to start and operate.
   - External CLIs (Claude/Codex) MUST be discoverable/configurable across common install methods.

## North Star Metrics & Acceptance Criteria
We use these to evaluate whether an iteration moves the product closer to the vision.

### North Star Metrics (observable/testable)
- **Time-to-start**: from clicking “New Run” to first visible log/event should be fast (target: <10s in a healthy local setup).
- **Continuity success rate**: resuming a valid session should succeed reliably; invalid resumes should fail fast with a clear reason and next-step guidance.
- **Stuck perception**: during a long run, users can always tell the system is working (active step shown, newest logs surfaced, no “is it frozen?” moments).
- **Safety coverage**: unsafe actions are consistently routed through the intended approval mode without accidental bypass.

### Acceptance Criteria (what “done” means per iteration)
- Every OpenSpec change MUST state which principle(s) and metric(s) it is improving.
- Every OpenSpec change MUST include a verification step (tests, validate, or a reproducible manual checklist).
- UX changes MUST include at least one narrow-screen/mobile check.

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
   - `add-delivery-foreman-check` (done)
   - `add-approval-trilogy` (done)

2) **Mobile Shell (interaction foundation)**
   - `add-mobile-shell` (done)

3) **Workspace File Ops (close the loop; allow safe writes)**
   - `add-workspace-file-ops` (done)

4) **Preview Tabs (Markdown/Raw/HTML preview)**
   - `add-preview-tabs` (done)

5) **Session Management (lower priority, but must exist)**
   - `add-session-management` (done)

6) **Machine Skills Management (tooling foundation)**
   - `add-machine-skills-management` (done)

7) **Tooling Extensibility (multi-tool adapters + tool-level env)**
   - `add-tooling-extensibility` (done)

8) **Foreman Dashboard (operational overview)**
   - `add-foreman-dashboard` (done)

### Next Changes Queue (dependency-ordered)
This is the next actionable queue after the completed phases above. New items MUST be appended here (and ordered) before implementation starts.

1) `add-context-and-templates`
   - Context panel, context compression, and reusable templates to reduce “blank prompt” work.

2) `add-quick-actions`
   - High-frequency cockpit shortcuts (one-click actions) for common workflows and “what’s next” guidance.

3) `add-file-advanced-capabilities`
   - File search, diff viewer, file monitor, upload/download (workbench-style; optional but valuable for power users).

4) `add-editor-monaco`
   - Monaco-grade editing for large files/language service/diff (workbench-style; optional; depends on 3).

### Timeline (Chronological, newest → oldest)
This is a time-ordered view for quickly understanding recent work. It does not replace dependency-driven ordering.

- `update-mobile-shell-polish` (done)
- `add-attention-autopilot` (done)
- `update-vision-metrics` (done)
- `update-secretary-drawer-ui` (done)
- `update-secretary-autoresume` (done)
- `update-dashboard-focus-polish` (done)
- `add-foreman-dashboard` (done)
- `add-tooling-extensibility` (done)
- `add-machine-skills-management` (done)
- `add-session-management` (done)
- `add-preview-tabs` (done)
- `add-workspace-file-ops` (done)
- `add-mobile-shell` (done)
- `add-approval-trilogy` (done)
- `add-delivery-foreman-check` (done)
- `add-claude-auto-approve-toggle` (done)
- `add-approval-blocked-mvp` (done)
- `add-controlccx-foundation` (done)

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
