# ControlCCX

Go + Vue “control center” for running multiple agent workers (Claude Code / Codex) asynchronously, with a built-in observer that can answer questions about current task state.

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
```

## Resume (断点接续)

Tasks and logs are persisted in SQLite. If the server exits while tasks are running, those tasks will appear as `interrupted` on next startup. You can resume by starting a new run using the persisted session/thread ID (UI has a “Resume” action when `session_id` exists).

