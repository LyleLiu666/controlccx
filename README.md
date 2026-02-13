# ControlCCX

Go + Vue “control center” for running multiple agent workers (Claude Code / Codex) asynchronously, with a built-in secretary agent (“系统级秘书”) for task/system state query and task mutation.

## Vision（愿景）

本项目只是一个让用户可以用得更舒服的盒子：我们不应该管太多，而是应该把 Claude Code 和 OpenAI Codex 用出最佳实践。

## One-command start

### Dev (API + UI, cross-platform)
```bash
pnpm install
pnpm dev
```

- UI: `http://127.0.0.1:5173`
- API: `http://127.0.0.1:5174`

### Production-like (single origin serves UI + API)
```bash
pnpm start
```

- Server: `http://127.0.0.1:5174`
- The web UI is embedded into the server binary after build (no runtime static directory required).
- The server will try to open the web UI in your default browser automatically (disable with `--no-open` or `CONTROLCCX_NO_OPEN=1`).

### Startup scripts

- macOS/Linux: `./start.sh`
- Windows (PowerShell): `powershell -ExecutionPolicy Bypass -File .\\start.ps1`

## Releases

When a GitHub Release is **published**, GitHub Actions builds and uploads binaries (with embedded web UI):

- `controlccx_<tag>_linux_amd64.tar.gz`
- `controlccx_<tag>_darwin_amd64.tar.gz`
- `controlccx_<tag>_windows_amd64.zip`
- `SHA256SUMS.txt`

## Tests

```bash
pnpm test
```

Manual acceptance helpers:

- Session queue behavior: `./scripts/manual_session_queue_acceptance.sh <session-key>`
- Secretary scheduler E2E (create -> auto-poll -> immediate callback reports): `./scripts/manual_secretary_scheduler_e2e.sh`
- Secretary scheduler visibility E2E (SSE + chat history consistency): `./scripts/manual_secretary_scheduler_visibility_e2e.sh`

## Configuration

Default data dir: `~/.controlccx/`

- DB: `~/.controlccx/controlccx.db`
- Config: `~/.controlccx/config.yaml`

Example `config.yaml`:

```yaml
server:
  addr: 127.0.0.1:5174
secretary:
  # Per LLM request timeout for the built-in secretary agent. Example: 30m / 90s.
  # Set to "0" to disable (not recommended; can hang until the client disconnects).
  llm_timeout: 30m
paths:
  claude: /path/to/claude
  codex: /path/to/codex
  # Windows only (Claude Code runs via Git Bash for consistency)
  git_bash: C:\Program Files\Git\bin\bash.exe
workers:
  # Default: false. When true, enables unattended "dangerously-*" flags
  # (e.g. Claude Code skip permissions, Codex bypass approvals/sandbox).
  unsafe_automation: false
```

You can also override the secretary LLM timeout via env var: `CONTROLCCX_SECRETARY_LLM_TIMEOUT=30m`.

## Worker authentication

Workers inherit environment variables from the ControlCCX server process. You can also set keys/tokens in the web UI (Settings), which persists them to `~/.controlccx/secrets.json` and injects them into newly started worker processes (env vars take precedence).

- Claude Code (API key): `ANTHROPIC_API_KEY`
- Claude Code (subscription token): `ANTHROPIC_AUTH_TOKEN` (or run `claude /login` once in a terminal on this machine)
- Codex: `OPENAI_API_KEY`

## Secretary（系统级秘书）

秘书支持两类能力：

- 查询（只读）：任务总数 / 已完成 / 失败 / 当前会话状态等
- 写操作：可通过秘书工具直接新建任务（`task_new_submit`）

写操作参数契约为显式模式：`worker_type`、`prompt`、`workdir` 为必填；缺参时秘书会先向用户索取，不做继承/猜测/自动补全。

秘书交互默认遵循“先理解再执行”的原则：

- 信息不足时一次只问一个关键问题，采用“多选引导 + 一句猜测”
- `worker_type` 仅允许：`claude-code` / `codex` / `exec`
- `worker_type` 语义：
  `claude-code` = Claude Code 代理执行
  `codex` = Codex 代理执行
  `exec` = 在本机 `workdir` 直接执行 shell/脚本（由 worker 进程执行，不是秘书自身执行）
- 选择建议：简单且追求速度优先 `claude-code`；严肃/生产级迭代优先 `codex`；不确定先问再提
- `exec` 不作为自动推荐项，仅在用户明确要求 shell/脚本执行时使用
- 写操作前先给一行执行摘要（目标/worker/验收），确认后再提交

- UI：右上角 `⋯` 菜单 → `秘书`
- API：
  - `GET  /api/secretary/messages?limit=200`
  - `POST /api/secretary/messages` body `{ "message": "..." }` → `{ "reply": "..." }`
  - `POST /api/secretary/clear` → `{ "ok": true }`
- 历史：服务端持久化到 SQLite `chat_messages`（全局 1 条历史，自动保留最近 2000 条）
- 旧接口：`/api/chat` 仍保持移除，统一使用 `/api/secretary/*`

## Task Mutation Contract（Breaking Change）

任务变更接口（create/resume/continue/preempt-continue/rehydrate/enter-unsafe）统一改为 envelope 结构。

- Success:
  - `{ "ok": true, "action": "...", "task": {...} }`
  - 或 `{ "ok": true, "action": "...", "queue": {...} }`
- Problem:
  - `{ "ok": false, "error": "...", "message": "...", "hint": "...", "details": {...} }`

`error` 稳定码包括：`invalid_argument`、`not_found`、`workdir_busy`、`session_task_in_flight`、`runner_unavailable`、`unsupported`、`internal`。

注意：旧的返回体格式（例如直接返回 `Task`、或冲突字段平铺在顶层）已移除。外部脚本调用者需迁移为读取 `ok + action + task/queue` 或 `ok=false + error + details`。

## Approvals

### Run Safety Autopilot（默认推荐）

默认开启：ControlCCX 会根据 prompt 推断任务意图（`analyze` / `code` / `search-browse` / `install`），并为 Claude Code / Codex 自动选择更稳妥的 sandbox/permissions 组合，尽量减少弹窗和人工选择。

- “Install unlock”（一次性解锁，高风险）：仅当你明确勾选后，系统才会把 `install` 类任务升级到更宽松/危险的模式（例如 Codex 的 `danger-full-access`、Claude 的 `--dangerously-skip-permissions`）。
- 未解锁时，`install` 任务会回退到更保守的 sandbox 默认值。

## Resume (断点接续)

Tasks and logs are persisted in SQLite. If the server exits while tasks are running, those tasks will appear as `interrupted` on next startup. You can resume by starting a new run using the persisted session/thread ID (UI has a “Resume” action when `session_id` exists).

### Claude Code: session 不可恢复（No conversation found）

Claude Code 的 session 由 Claude 自己在本机维护。某些情况下（例如更换工作目录作用域、Claude 侧清理会话、登录上下文变化等），会出现：

```
No conversation found with session ID: ...
```

此时 Resume 无法继续。ControlCCX 提供一个降级路径（Rehydrate）：把该 session 下已持久化的 prompt/assistant 输出拼接成上下文，用 **New Run（mode=new）** 开一个新的会话继续任务（不会复用旧 `session_id`）。

注意：如果该会话存在隔离工作区（Run Workspace）且状态为 `active`，请先在 UI 的 Workspace 面板执行 Merge，把改动合并回 `base_workdir`，再进行 Rehydrate，避免丢改动。
