# ControlCCX

Go + Vue “control center” for running multiple agent workers (Claude Code / Codex) asynchronously, with a built-in observer that can answer questions about current task state.

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

## Configuration

Default data dir: `~/.controlccx/`

- DB: `~/.controlccx/controlccx.db`
- Config: `~/.controlccx/config.yaml`

Example `config.yaml`:

```yaml
server:
  addr: 127.0.0.1:5174
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

## Worker authentication

Workers inherit environment variables from the ControlCCX server process. You can also set keys/tokens in the web UI (Settings), which persists them to `~/.controlccx/secrets.json` and injects them into newly started worker processes (env vars take precedence).

- Claude Code (API key): `ANTHROPIC_API_KEY`
- Claude Code (subscription token): `ANTHROPIC_AUTH_TOKEN` (or run `claude /login` once in a terminal on this machine)
- Codex: `OPENAI_API_KEY`

## Secretary (Observer) is LLM-only (禁止 deterministic/heuristic)

The built-in Secretary (Observer) is an **agentic** role: it MUST use an LLM backend (Claude Code CLI / Codex CLI) and tools to reason, inspect real system state, and perform actions.

ControlCCX intentionally does **not** provide deterministic/heuristic “fallback answers” for the Secretary (e.g. “count tasks from DB without LLM”, or “auto resume when LLM is missing”). If the LLM backend is not configured/available, the Secretary will fail-fast and tell you the minimal fix steps.

Claude Code compatible vendors (e.g. Kimi/Minimax/GLM gateways) are supported via standard env vars:

- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_AUTH_TOKEN` (required to support)
- `ANTHROPIC_MODEL`

## Approvals (三档审批策略)

部分 worker（尤其是 Claude Code）在调用工具/执行敏感动作时会要求 approval。ControlCCX 计划使用“驾驶室 + 秘书（Secretary）”的三档审批策略：

### Run Safety Autopilot（默认推荐）

默认开启：ControlCCX 会根据 prompt 推断任务意图（`analyze` / `code` / `search-browse` / `install`），并为 Claude Code / Codex 自动选择更稳妥的 sandbox/permissions 组合，尽量减少弹窗和人工选择。

- “Install unlock”（一次性解锁，高风险）：仅当你明确勾选后，系统才会把 `install` 类任务升级到更宽松/危险的模式（例如 Codex 的 `danger-full-access`、Claude 的 `--dangerously-skip-permissions`）。
- 未解锁时，`install` 任务会回退到更保守的 sandbox 默认值。

1) Level 1（直通）：完全不需要审批，全部自动通过（效率最高、风险最高）。
   - 适用：你明确接受风险，希望不中断。
   - 风险：可能在未提示的情况下执行危险命令（例如删除文件、远端 push、安装脚本）。

2) Level 2（秘书全审）：所有审批都交给秘书自动决策（用户不被频繁打断）。
   - 适用：你信任秘书，追求不中断，同时希望避免明显危险操作。
   - 约束：秘书的自动决策必须可追溯（system log 可回看）。

3) Level 3（秘书优先 + 升级）：秘书先决策；遇到高风险/不确定时再升级给用户确认。
   - 适用：默认安全、只在关键时刻打断你。

“升级给用户”常见触发示例（2～4 条）：
- 大范围删除/破坏性改动（例如 `rm -rf`、删除大量文件、清空目录）
- 远端/网络敏感操作（例如 `git push/pull/fetch/remote`、`curl/wget` 到陌生域名）
- 可能泄露 secrets 的操作（例如打印 env、读取 secrets 文件、上传日志到远端）
- 执行未知脚本/安装依赖链（例如运行安装脚本、执行未知二进制）

注意：不提供“所有决策都必须用户审批”的模式；New Run 与 Resume 使用同一策略。

当前状态（2026-01-28）：
- 已实现 blocked 基础能力：当 CLI 要求 approval 时，任务会进入 `blocked`（避免被误判为 failed）。
- 已提供 Level 1 相关开关：全局 `workers.unsafe_automation` + UI 的 Auto-approve（危险）。
- Level 2/3 的完整链路（秘书全审/升级面板）按 OpenSpec 分阶段实现中。

## Resume (断点接续)

Tasks and logs are persisted in SQLite. If the server exits while tasks are running, those tasks will appear as `interrupted` on next startup. You can resume by starting a new run using the persisted session/thread ID (UI has a “Resume” action when `session_id` exists).

### Claude Code: session 不可恢复（No conversation found）

Claude Code 的 session 由 Claude 自己在本机维护。某些情况下（例如更换工作目录作用域、Claude 侧清理会话、登录上下文变化等），会出现：

```
No conversation found with session ID: ...
```

此时 Resume 无法继续。ControlCCX 提供一个降级路径（Rehydrate）：把该 session 下已持久化的 prompt/assistant 输出拼接成上下文，用 **New Run（mode=new）** 开一个新的会话继续任务（不会复用旧 `session_id`）。

注意：如果该会话存在隔离工作区（Run Workspace）且状态为 `active`，请先在 UI 的 Workspace 面板执行 Merge，把改动合并回 `base_workdir`，再进行 Rehydrate，避免丢改动。
