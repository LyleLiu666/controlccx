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
- The web UI is embedded into the server binary after build (no runtime static directory required).

### Startup scripts

- macOS/Linux: `./start.sh`
- Windows (PowerShell): `powershell -ExecutionPolicy Bypass -File .\\start.ps1`

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

## Resume (断点接续)

Tasks and logs are persisted in SQLite. If the server exits while tasks are running, those tasks will appear as `interrupted` on next startup. You can resume by starting a new run using the persisted session/thread ID (UI has a “Resume” action when `session_id` exists).
