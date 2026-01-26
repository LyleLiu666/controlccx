package worker

import (
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

	args := []string{"-p"}
	// Default to skip permissions for consistent automation. Can be made configurable later.
	args = append(args, "--dangerously-skip-permissions")
	// Note: do not force --setting-sources. Users often rely on their normal Claude settings/auth.
	if task.Mode == tasks.ModeResume && strings.TrimSpace(task.SessionID) != "" {
		args = append(args, "-r", strings.TrimSpace(task.SessionID))
	}
	args = append(args, "--output-format", "stream-json", "--verbose", "-")

	tool := ToolCommand{
		Command: cmd,
		Args:    args,
		Dir:     workdir,
		Stdin:   task.Prompt,
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
		tool.Stdin = task.Prompt
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

	args := []string{"e"}
	// Default to bypass approvals/sandbox for consistent automation. Can be made configurable later.
	args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	args = append(args, "--skip-git-repo-check")

	if task.Mode == tasks.ModeResume && strings.TrimSpace(task.SessionID) != "" {
		args = append(args, "--json", "resume", strings.TrimSpace(task.SessionID), "-")
	} else {
		args = append(args, "-C", workdir, "--json", "-")
	}

	tool := ToolCommand{
		Command: cmd,
		Args:    args,
		Dir:     workdir,
		Stdin:   task.Prompt,
	}

	if runtime.GOOS == "windows" {
		tool.Warning = "codex on Windows is best-effort (PowerShell environment can be unstable)"
	}
	return tool, nil
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
