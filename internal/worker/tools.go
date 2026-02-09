package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"controlccx/internal/config"
	"controlccx/internal/tasks"
)

type ToolCommand struct {
	Command string
	Args    []string
	Dir     string
	Stdin   string
	Warning string
}

const worktreePromptSentinel = "CCX_WORKTREE_MODE"
const workspacePromptSentinel = "CCX_WORKSPACE_MODE"

func effectivePromptForTask(task tasks.Task) string {
	prompt := task.Prompt
	strategy := strings.ToLower(strings.TrimSpace(task.WorkDirStrategy))
	if strategy != "worktree" && strategy != "workspace" {
		return prompt
	}
	if strings.TrimSpace(prompt) == "" {
		return prompt
	}
	if strings.Contains(prompt, worktreePromptSentinel) || strings.Contains(prompt, workspacePromptSentinel) {
		return prompt
	}

	base := strings.TrimSpace(task.BaseWorkDir)
	run := strings.TrimSpace(task.WorktreeDir)
	branch := strings.TrimSpace(task.WorktreeBranch)

	header := ""
	switch strategy {
	case "worktree":
		header = strings.TrimSpace(fmt.Sprintf(
			`[%s]
你正在一个 Git worktree 中执行（并发开发模式）。

BaseWorkDir（不要直接改这里）: %s
WorktreeDir（当前工作目录）: %s
WorktreeBranch: %s

硬性规则（必须遵守）：
1) 只修改 WorktreeDir 下的文件；禁止修改 BaseWorkDir 下的文件。
2) 所有 git 操作都在 WorktreeDir 中进行（或使用 git -C WorktreeDir）。
3) 不要删除/移动 worktree 目录（.ccx/worktrees/...）。

完成后请输出：
- `+"`git status`"+`（worktree）
- 关键改动摘要
- 合并回 base repo 的建议步骤（如需）`,
			worktreePromptSentinel,
			base,
			run,
			branch,
		))
	case "workspace":
		kind := strings.TrimSpace(task.RunWorkspaceKind)
		baseBranch := strings.TrimSpace(task.RunWorkspaceBaseBranch)
		workBranch := strings.TrimSpace(task.RunWorkspaceWorkBranch)
		lines := []string{
			fmt.Sprintf("[%s]", workspacePromptSentinel),
			"你正在一个隔离的 run workspace 中执行（会话隔离模式）。",
			"",
			fmt.Sprintf("BaseWorkDir（不要直接改这里）: %s", base),
			fmt.Sprintf("RunWorkDir（当前工作目录）: %s", run),
		}
		if kind != "" {
			lines = append(lines, fmt.Sprintf("WorkspaceKind: %s", kind))
		}
		if baseBranch != "" {
			lines = append(lines, fmt.Sprintf("BaseBranch: %s", baseBranch))
		}
		if workBranch != "" {
			lines = append(lines, fmt.Sprintf("WorkBranch: %s", workBranch))
		}
		lines = append(lines,
			"",
			"硬性规则（必须遵守）：",
			"1) 只修改 RunWorkDir 下的文件；禁止修改 BaseWorkDir 下的文件。",
			"2) 不要删除/移动 run workspace 目录（.ccx/workspaces/...）。",
		)
		header = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if header == "" {
		return prompt
	}
	return header + "\n\n---\n\n" + prompt
}

func BuildToolCommand(cfg config.Config, task tasks.Task) (ToolCommand, error) {
	switch task.WorkerType {
	case tasks.WorkerClaudeCode:
		return buildClaude(cfg, task)
	case tasks.WorkerCodex:
		return buildCodex(cfg, task)
	case tasks.WorkerExec:
		return buildExec(cfg, task), nil
	default:
		return ToolCommand{}, ErrUnsupportedWorkerType{WorkerType: task.WorkerType}
	}
}

type ErrUnsupportedWorkerType struct {
	WorkerType tasks.WorkerType
}

func (e ErrUnsupportedWorkerType) Error() string {
	return "unsupported worker_type: " + string(e.WorkerType)
}

func buildClaude(cfg config.Config, task tasks.Task) (ToolCommand, error) {
	cmd := strings.TrimSpace(cfg.Paths.Claude)
	if cmd == "" {
		cmd = "claude"
	}
	workdir := filepath.Clean(task.WorkDir)
	prompt := effectivePromptForTask(task)
	if normalized, changes := normalizePromptSkillTokensForExecution(task.WorkerType, prompt); changes > 0 {
		prompt = normalized
	}

	args := []string{"-p"}
	if cfg.Workers.UnsafeAutomation || task.UnsafeAutomation {
		args = append(args, "--dangerously-skip-permissions")
	}
	if strings.TrimSpace(task.ClaudePermissionMode) != "" {
		args = append(args, "--permission-mode", strings.TrimSpace(task.ClaudePermissionMode))
	}
	if settings, ok := claudeSettingsForTask(task); ok {
		args = append(args, "--settings", settings)
	}
	// Note: do not force --setting-sources. Users often rely on their normal Claude settings/auth.
	if task.Mode == tasks.ModeResume && strings.TrimSpace(task.SessionID) != "" {
		args = append(args, "-r", strings.TrimSpace(task.SessionID))
	}
	args = append(args,
		"--permission-prompt-tool=stdio",
		"--verbose",
		"--output-format=stream-json",
		"--input-format=stream-json",
		"--include-partial-messages",
		"--disallowedTools=AskUserQuestion",
	)
	if strings.TrimSpace(os.Getenv("CONTROLCCX_TEST_CLAUDE_HELPER_PROCESS")) != "" {
		// Test-only escape hatch: allow worker tests to use the Go test binary as a fake Claude CLI.
		// The helper process can access the original CLI args via flag.Args() after "--".
		args = append([]string{"-test.run=TestCCXClaudeHelperProcess", "--", "ccx-helper-claude"}, args...)
	}

	tool := ToolCommand{
		Command: cmd,
		Args:    args,
		Dir:     workdir,
		Stdin:   prompt,
	}

	// On Windows, run Claude Code via Git Bash for more consistent behavior.
	if runtime.GOOS == "windows" {
		gitBash := strings.TrimSpace(cfg.Paths.GitBash)
		if gitBash == "" {
			return ToolCommand{}, ErrMissingGitBash{}
		}
		// Use stdin for the prompt to avoid quoting issues.
		quoted := shellJoin(cmd, args)
		tool.Command = gitBash
		tool.Args = []string{"-lc", quoted}
		tool.Stdin = prompt
	}

	return tool, nil
}

type ErrMissingGitBash struct{}

func (ErrMissingGitBash) Error() string {
	return "git_bash path is required on Windows for claude-code"
}

func buildCodex(cfg config.Config, task tasks.Task) (ToolCommand, error) {
	cmd := strings.TrimSpace(cfg.Paths.Codex)
	if cmd == "" {
		cmd = "codex"
	}

	workdir := filepath.Clean(task.WorkDir)
	prompt := effectivePromptForTask(task)
	if normalized, changes := normalizePromptSkillTokensForExecution(task.WorkerType, prompt); changes > 0 {
		prompt = normalized
	}

	unsafe := cfg.Workers.UnsafeAutomation || task.UnsafeAutomation

	args := []string{}
	if task.CodexSearch {
		// Codex feature flags are opt-in; search is disabled by default.
		args = append(args, "--enable", "web_search_request")
	}
	if unsafe {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	args = append(args, "app-server")
	if strings.TrimSpace(os.Getenv("CONTROLCCX_TEST_CODEX_HELPER_PROCESS")) != "" {
		// Test-only escape hatch: allow worker tests to use the Go test binary as a fake Codex app-server.
		// The helper process can access the original CLI args via flag.Args() after "--".
		args = append([]string{"-test.run=TestCCXCodexHelperProcess", "--", "ccx-helper-codex"}, args...)
	}

	tool := ToolCommand{
		Command: cmd,
		Args:    args,
		Dir:     workdir,
		Stdin:   prompt,
	}

	if runtime.GOOS == "windows" {
		tool.Warning = "codex on Windows is best-effort (PowerShell environment can be unstable)"
	}
	return tool, nil
}

func claudeSettingsForTask(task tasks.Task) (string, bool) {
	// We only inject settings when explicitly requested. This avoids implicitly changing
	// behavior for existing users/API clients that rely on their own Claude settings.
	if !task.ClaudeSandbox && len(task.ClaudeWebFetchDomains) == 0 && strings.TrimSpace(task.SafetyPreset) == "" && strings.TrimSpace(task.TaskIntent) == "" {
		return "", false
	}

	preset := strings.ToLower(strings.TrimSpace(task.SafetyPreset))
	noNetwork := strings.Contains(preset, "no-network")
	unsafe := strings.Contains(preset, "unsafe") || task.UnsafeAutomation
	taskIntent := strings.ToLower(strings.TrimSpace(task.TaskIntent))
	allowCurlWget := !noNetwork && (taskIntent == "search-browse" || (taskIntent == "" && strings.Contains(preset, "search-browse")))

	type permissions struct {
		Allow []string `json:"allow,omitempty"`
		Ask   []string `json:"ask,omitempty"`
		Deny  []string `json:"deny,omitempty"`
	}

	p := permissions{
		Deny: []string{
			"Read(./.env)",
			"Read(./secrets/**)",
		},
	}
	if !unsafe && !allowCurlWget {
		p.Deny = append(p.Deny, "Bash(curl *)", "Bash(wget *)")
	}
	if noNetwork {
		p.Deny = append(p.Deny, "WebFetch", "WebSearch")
	} else {
		if len(task.ClaudeWebFetchDomains) > 0 {
			for _, d := range task.ClaudeWebFetchDomains {
				d = strings.TrimSpace(d)
				if d == "" {
					continue
				}
				p.Allow = append(p.Allow, "WebFetch(domain:"+d+")")
			}
		} else {
			// Search/browse is considered low-risk. Allow WebFetch without requiring a domain allowlist.
			p.Allow = append(p.Allow, "WebFetch")
		}
		// WebSearch is safe-by-default for non-interactive search/browse runs.
		p.Allow = append(p.Allow, "WebSearch")
	}

	type sandboxSettings struct {
		Enabled                  bool  `json:"enabled"`
		AutoAllowBashIfSandboxed bool  `json:"autoAllowBashIfSandboxed,omitempty"`
		AllowUnsandboxedCommands *bool `json:"allowUnsandboxedCommands,omitempty"`
	}

	var sandbox *sandboxSettings
	if runtime.GOOS != "windows" {
		if unsafe {
			// Unsafe mode is an explicit opt-in to higher risk. Disable Claude's bash sandbox so
			// networking (pip/python/curl) works as expected for installs and other automation.
			sandbox = &sandboxSettings{Enabled: false}
		} else if task.ClaudeSandbox {
			allowUnsandboxedCommands := false
			sandbox = &sandboxSettings{
				Enabled:                  true,
				AutoAllowBashIfSandboxed: true,
				AllowUnsandboxedCommands: &allowUnsandboxedCommands,
			}
		}
	}

	payload := map[string]any{
		"permissions": p,
	}
	if sandbox != nil {
		payload["sandbox"] = sandbox
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func buildExec(cfg config.Config, task tasks.Task) ToolCommand {
	workdir := filepath.Clean(task.WorkDir)
	cmd := strings.TrimSpace(task.Prompt)
	tool := ToolCommand{Dir: workdir}
	if runtime.GOOS == "windows" {
		tool.Command = "cmd.exe"
		tool.Args = []string{"/c", cmd}
		return tool
	}
	tool.Command = "sh"
	tool.Args = []string{"-lc", cmd}
	return tool
}

func shellJoin(command string, args []string) string {
	// Minimal shell escaping for bash -lc; prompts are passed via stdin so args are fixed.
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, escapeBash(command))
	for _, a := range args {
		parts = append(parts, escapeBash(a))
	}
	return strings.Join(parts, " ")
}

func escapeBash(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`") {
		return s
	}
	// Single-quote style: 'foo'"'"'bar'
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
