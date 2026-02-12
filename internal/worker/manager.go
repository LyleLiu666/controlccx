package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/events"
	"controlccx/internal/execenv"
	"controlccx/internal/runworkspace"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"

	"github.com/google/uuid"
)

type Manager struct {
	cfg   config.Config
	store *tasks.Store
	hub   *events.Hub
	auth  *auth.Store
	tools *tooling.Service
	ws    *runworkspace.Service

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	updateMu       sync.Mutex
	lastTaskUpdate map[string]time.Time

	approvalMu      sync.Mutex
	approvalWaiters map[string]approvalWaiter
	approvalTimeout time.Duration
}

const workspaceRequiredWarningPrefix = "CCX_WORKSPACE_REQUIRED:"

func shouldUseRunWorkspace(task tasks.Task, hasExistingWorkspace bool, initProject bool) (bool, string) {
	if hasExistingWorkspace {
		return true, "existing-workspace"
	}
	if initProject {
		return false, "init-project"
	}

	// workdir_strategy=worktree already executes inside an isolated worktree.
	if strings.ToLower(strings.TrimSpace(task.WorkDirStrategy)) == "worktree" {
		return false, "worktree-strategy"
	}

	// Exec runs are unsafe to execute in the base workdir by default.
	if task.WorkerType == tasks.WorkerExec {
		return true, "exec-default"
	}

	intent := strings.ToLower(strings.TrimSpace(task.TaskIntent))
	switch intent {
	case "analyze", "search-browse":
		return false, "intent:" + intent
	case "code", "install":
		return true, "intent:" + intent
	case "":
		// Safe default: preserve legacy behavior for missing intents.
		return true, "intent:default"
	default:
		// Unknown intents are treated as code-like to preserve safety.
		return true, "intent:" + intent
	}
}

func detectGitRepoRoot(ctx context.Context, dir string) (string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false
	}

	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	return filepath.Clean(root), true
}

func gitHasHEAD(ctx context.Context, repoRoot string) bool {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "--verify", "HEAD")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func isInitProject(ctx context.Context, baseWorkDir string) bool {
	baseWorkDir = filepath.Clean(strings.TrimSpace(baseWorkDir))
	if baseWorkDir == "" {
		baseWorkDir = "."
	}

	if repoRoot, ok := detectGitRepoRoot(ctx, baseWorkDir); ok {
		// Git repo without any commits (unborn HEAD).
		return !gitHasHEAD(ctx, repoRoot)
	}

	// Non-git: treat an empty top-level directory as "init project".
	entries, err := os.ReadDir(baseWorkDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if name == "" {
			continue
		}
		switch name {
		case ".git", ".ccx", ".DS_Store":
			continue
		default:
			return false
		}
	}
	return true
}

func NewManager(cfg config.Config, store *tasks.Store, hub *events.Hub, authStore *auth.Store, tools *tooling.Service) *Manager {
	var ws *runworkspace.Service
	if store != nil {
		ws = runworkspace.NewService(store, runworkspace.Options{})
	}
	return &Manager{
		cfg:     cfg,
		store:   store,
		hub:     hub,
		auth:    authStore,
		tools:   tools,
		ws:      ws,
		cancels: make(map[string]context.CancelFunc),

		approvalWaiters: make(map[string]approvalWaiter),
		approvalTimeout: 5 * time.Minute,
	}
}

func (m *Manager) Start(ctx context.Context, taskID string) error {
	if m.store == nil {
		return errors.New("worker: store is required")
	}

	task, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != tasks.StatusQueued {
		return fmt.Errorf("worker: task not queued: %s (status=%s)", taskID, task.Status)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if m.cancels == nil {
		m.cancels = make(map[string]context.CancelFunc)
	}
	if _, exists := m.cancels[taskID]; exists {
		m.mu.Unlock()
		cancel()
		return fmt.Errorf("worker: task already running: %s", taskID)
	}
	m.cancels[taskID] = cancel
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.cancels, taskID)
			m.mu.Unlock()

			m.updateMu.Lock()
			delete(m.lastTaskUpdate, taskID)
			m.updateMu.Unlock()
		}()
		_ = m.run(runCtx, task)
	}()
	return nil
}

func (m *Manager) Cancel(ctx context.Context, taskID string) (bool, error) {
	m.mu.Lock()
	cancel, ok := m.cancels[taskID]
	m.mu.Unlock()
	if !ok {
		return false, nil
	}
	cancel()
	return true, nil
}

type approvalDecision struct {
	Decision string
	Reason   string
}

type approvalWaiter struct {
	TaskID string
	C      chan approvalDecision
}

// SubmitApprovalDecision notifies a running task that an approval was decided.
// Returns true if a waiting run was notified.
func (m *Manager) SubmitApprovalDecision(taskID string, approvalID string, decision string, reason string) bool {
	if m == nil {
		return false
	}
	taskID = strings.TrimSpace(taskID)
	approvalID = strings.TrimSpace(approvalID)
	decision = strings.ToLower(strings.TrimSpace(decision))
	reason = strings.TrimSpace(reason)
	if taskID == "" || approvalID == "" {
		return false
	}
	if decision != "approve" && decision != "deny" {
		return false
	}

	m.approvalMu.Lock()
	waiter, ok := m.approvalWaiters[approvalID]
	m.approvalMu.Unlock()
	if !ok {
		return false
	}
	if strings.TrimSpace(waiter.TaskID) != taskID {
		return false
	}

	select {
	case waiter.C <- approvalDecision{Decision: decision, Reason: reason}:
		return true
	default:
		// Already decided or receiver gone.
		return false
	}
}

func (m *Manager) run(ctx context.Context, task tasks.Task) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := m.store.SetRunning(context.Background(), task.ID); err != nil {
		return err
	}
	m.publishTaskUpdated(task.ID)

	heartbeatStop := make(chan struct{})
	defer close(heartbeatStop)
	if m != nil && m.store != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-heartbeatStop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = m.store.TouchTask(context.Background(), task.ID)
				}
			}
		}()
	}

	// Run workspace: execute inside a session-scoped isolated directory (run_workdir).
	effective := task
	initProject := false
	runWorkspaceActive := false
	if m != nil && m.ws != nil {
		if strings.ToLower(strings.TrimSpace(task.WorkDirStrategy)) != "worktree" {
			initProject = isInitProject(ctx, task.WorkDir)

			key := strings.TrimSpace(tasks.SessionKeyForTask(task))
			hasExistingWorkspace := false
			if key != "" {
				if ws, ok, err := m.ws.Get(ctx, key); err == nil && ok {
					if strings.TrimSpace(ws.RunWorkDir) != "" {
						if _, err := os.Stat(strings.TrimSpace(ws.RunWorkDir)); err == nil {
							hasExistingWorkspace = true
						}
					}
				}
			}

			useWorkspace, reason := shouldUseRunWorkspace(task, hasExistingWorkspace, initProject)
			if useWorkspace {
				ensureStart := time.Now()
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace: ensure start base=%s", filepath.Clean(task.WorkDir)))
				ens, err := m.ws.EnsureForTask(ctx, task)
				if err != nil {
					m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace setup error: %v", err))
					_ = m.store.FinishTask(context.Background(), task.ID, tasks.FinishTaskInput{
						Status:     tasks.StatusFailed,
						ExitCode:   nil,
						Error:      err.Error(),
						SessionID:  "",
						FinishedAt: time.Now().UTC(),
					})
					m.publishTaskUpdated(task.ID)
					return err
				}
				ws := ens.Workspace
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace: ensure done kind=%s duration_ms=%d", strings.TrimSpace(ws.Kind), time.Since(ensureStart).Milliseconds()))
				if strings.TrimSpace(ws.RunWorkDir) != "" {
					effective.WorkDir = strings.TrimSpace(ws.RunWorkDir)
				}
				effective.WorkDirStrategy = "workspace"
				effective.BaseWorkDir = strings.TrimSpace(ws.BaseWorkDir)
				effective.WorktreeDir = strings.TrimSpace(ws.RunWorkDir)
				effective.WorktreeBranch = strings.TrimSpace(ws.WorkBranch)
				effective.RunWorkspaceKind = strings.TrimSpace(ws.Kind)
				effective.RunWorkspaceBaseBranch = strings.TrimSpace(ws.BaseBranch)
				effective.RunWorkspaceWorkBranch = strings.TrimSpace(ws.WorkBranch)
				runWorkspaceActive = true

				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace: kind=%s base=%s run=%s", strings.TrimSpace(ws.Kind), filepath.Clean(ws.BaseWorkDir), filepath.Clean(ws.RunWorkDir)))
				if strings.TrimSpace(reason) != "" {
					m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace: decision=%s", strings.TrimSpace(reason)))
				}
				for _, msg := range ens.Logs {
					msg = strings.TrimSpace(msg)
					if msg == "" {
						continue
					}
					m.appendLog(task.ID, tasks.LogSystem, msg)
				}
			} else {
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace: skipped reason=%s", strings.TrimSpace(reason)))
			}
		}
	}

	tool, driver, err := m.buildToolCommand(ctx, effective)
	if err != nil {
		m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("worker setup error: %v", err))
		_ = m.store.FinishTask(context.Background(), task.ID, tasks.FinishTaskInput{
			Status:     tasks.StatusFailed,
			ExitCode:   nil,
			Error:      err.Error(),
			SessionID:  "",
			FinishedAt: time.Now().UTC(),
		})
		m.publishTaskUpdated(task.ID)
		return err
	}

	if tool.Warning != "" {
		_ = m.store.SetWarning(context.Background(), task.ID, tool.Warning)
		m.appendLog(task.ID, tasks.LogSystem, tool.Warning)
		m.publishTaskUpdated(task.ID)
	}

	cmd := exec.CommandContext(ctx, tool.Command, tool.Args...)
	cmd.Dir = tool.Dir
	env, injectedEnvKeys := m.envForToolWithReport(task.WorkerType, driver)
	cmd.Env = env

	// Persist trace metadata (best-effort; should not block execution).
	if m != nil && m.store != nil {
		if err := m.store.SetInvocation(context.Background(), task.ID, tool.Command, tool.Args, tool.Dir, injectedEnvKeys); err != nil {
			m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("trace warning: failed to persist invocation: %v", err))
		}
	}

	m.appendLog(task.ID, tasks.LogSystem, formatRunStartLog(task.WorkerType, driver, tool, injectedEnvKeys))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m.failTask(task.ID, fmt.Errorf("stdout pipe: %w", err))
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return m.failTask(task.ID, fmt.Errorf("stderr pipe: %w", err))
	}
	var (
		claudePeer  *claudeProtocolPeer
		claudeStdin io.WriteCloser
		codexPeer   *codexAppServerPeer
		codexStdin  io.WriteCloser
	)
	if driver == tasks.WorkerClaudeCode {
		claudeStdin, err = cmd.StdinPipe()
		if err != nil {
			return m.failTask(task.ID, fmt.Errorf("stdin pipe: %w", err))
		}
		claudePeer = newClaudeProtocolPeer(claudeStdin)
	} else if driver == tasks.WorkerCodex {
		codexStdin, err = cmd.StdinPipe()
		if err != nil {
			return m.failTask(task.ID, fmt.Errorf("stdin pipe: %w", err))
		}
		codexPeer = newCodexAppServerPeer(codexStdin)
	} else if tool.Stdin != "" {
		cmd.Stdin = stringsReader(tool.Stdin)
	}

	if err := cmd.Start(); err != nil {
		if isExecutableNotFound(err) {
			m.appendLog(task.ID, tasks.LogSystem, missingExecutableHint(tool.Command, driver))
		}
		return m.failTask(task.ID, fmt.Errorf("start: %w", err))
	}
	if claudePeer != nil {
		defer func() { _ = claudePeer.CloseStdin() }()
	}
	if codexPeer != nil {
		defer func() { _ = codexPeer.CloseStdin() }()
	}

	var (
		lastSessionIDMu sync.Mutex
		lastSessionID   string
	)
	resumeFailure := &resumeFailureState{}
	blockedState := &blockedState{}
	toolErrors := &toolErrorState{}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.consumeStdout(ctx, task, driver, stdout, claudePeer, codexPeer, runWorkspaceActive, initProject, &lastSessionIDMu, &lastSessionID, cancel, resumeFailure, blockedState, toolErrors)
	}()

	go func() {
		defer wg.Done()
		m.consumeLines(task, driver, tasks.LogStderr, stderr, cancel, resumeFailure, blockedState)
	}()

	if driver == tasks.WorkerClaudeCode && claudePeer != nil {
		prompt := tool.Stdin
		permissionMode := normalizeClaudePermissionMode(task.ClaudePermissionMode)
		go func() {
			// Note: do not fail the run if these protocol messages fail; surface as logs.
			if err := claudePeer.SendInitialize(uuid.NewString(), nil); err != nil {
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("claude protocol init error: %v", err))
				return
			}
			if err := claudePeer.SendSetPermissionMode(uuid.NewString(), permissionMode); err != nil {
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("claude protocol set_permission_mode error: %v", err))
			}
			if err := claudePeer.SendUserMessage(prompt); err != nil {
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("claude protocol send user message error: %v", err))
			}
		}()
	}
	if driver == tasks.WorkerCodex && codexPeer != nil {
		prompt := tool.Stdin
		go m.runCodexAppServer(ctx, task, codexPeer, prompt, resumeFailure, cancel)
	}

	wg.Wait()
	waitErr := cmd.Wait()

	exitCode := exitCode(waitErr)
	status := tasks.StatusSucceeded
	errText := ""
	if errors.Is(ctx.Err(), context.Canceled) {
		status = tasks.StatusCanceled
	} else if waitErr != nil {
		status = tasks.StatusFailed
		errText = waitErr.Error()
	}

	if failed, resumeMsg := resumeFailure.get(); failed {
		status = tasks.StatusFailed
		errText = resumeMsg
	} else if blocked, reason := blockedState.get(); blocked {
		// If we detected an approval-required prompt, treat this run as blocked even if
		// the underlying process exits non-zero.
		if status != tasks.StatusCanceled {
			status = tasks.StatusBlocked
			errText = ""
			if strings.TrimSpace(reason) != "" {
				_ = m.store.SetWarning(context.Background(), task.ID, reason)
			}
		}
	} else {
		// If stdout parsing already marked this run as blocked, preserve that status even if
		// the underlying process exited non-zero (common for non-interactive approval prompts).
		if status != tasks.StatusCanceled {
			if latest, err := m.store.GetTask(context.Background(), task.ID); err == nil && latest.Status == tasks.StatusBlocked {
				status = tasks.StatusBlocked
				errText = ""
			}
		}
	}

	m.appendLog(task.ID, tasks.LogSystem, formatRunFinishLog(status, exitCode, errText))

	lastSessionIDMu.Lock()
	sid := lastSessionID
	lastSessionIDMu.Unlock()

	sidToPersist, sidWarn := sessionIDToPersist(task, sid)
	if sidWarn != "" {
		m.appendLog(task.ID, tasks.LogSystem, sidWarn)
	}

	_ = m.store.FinishTask(context.Background(), task.ID, tasks.FinishTaskInput{
		Status:     status,
		ExitCode:   exitCode,
		Error:      errText,
		SessionID:  sidToPersist,
		FinishedAt: time.Now().UTC(),
	})

	if status == tasks.StatusSucceeded && toolErrors.seenAny() {
		latest, err := m.store.GetTask(context.Background(), task.ID)
		if err == nil && latest.Status == tasks.StatusSucceeded {
			merged := mergeWarning(latest.Warning, succeededWithToolErrorsWarning)
			if merged != strings.TrimSpace(latest.Warning) && merged != "" {
				_ = m.store.SetWarning(context.Background(), task.ID, merged)
			}
		}
	}
	m.publishTaskUpdated(task.ID)

	m.maybeStartNextWaitingForWorkdir(task.WorkDir)
	return nil
}

func sessionIDToPersist(task tasks.Task, observed string) (string, string) {
	observed = strings.TrimSpace(observed)
	requested := strings.TrimSpace(task.SessionID)
	if task.Mode != tasks.ModeResume {
		return observed, ""
	}
	// For resume tasks, keep the requested session_id stable for grouping.
	// Some CLIs may emit a new session id when resume fails or falls back.
	if requested != "" && observed != "" && observed != requested {
		return "", fmt.Sprintf("resume warning: session_id changed (requested=%q observed=%q). Keeping requested session_id for this run.", requested, observed)
	}
	return observed, ""
}

func (m *Manager) maybeStartNextWaitingForWorkdir(workdir string) {
	if m == nil || m.store == nil {
		return
	}

	next, ok, err := m.store.DequeueNextWaitingForWorkdir(context.Background(), workdir)
	if err != nil || !ok {
		return
	}

	_, _ = m.store.AppendLog(context.Background(), next.ID, tasks.LogSystem, fmt.Sprintf("wait-queue: workdir available; starting after previous run finished (workdir=%s)", filepath.Clean(workdir)))
	m.publishTaskUpdatedForce(next.ID)
	if err := m.Start(context.Background(), next.ID); err != nil {
		_ = m.store.FinishTask(context.Background(), next.ID, tasks.FinishTaskInput{
			Status:     tasks.StatusFailed,
			ExitCode:   nil,
			Error:      err.Error(),
			SessionID:  "",
			FinishedAt: time.Now().UTC(),
		})
		m.publishTaskUpdatedForce(next.ID)
	}
}

func (m *Manager) buildToolCommand(ctx context.Context, task tasks.Task) (ToolCommand, tasks.WorkerType, error) {
	driver := task.WorkerType
	cfg := m.cfg
	extraArgs := []string(nil)

	if m != nil && m.tools != nil {
		toolID := strings.TrimSpace(string(task.WorkerType))
		// "exec" is a built-in worker type and does not require a configurable tool profile.
		// Tooling is only used for external CLIs (claude-code / codex).
		if toolID != "" && toolID != string(tasks.WorkerExec) {
			profile, ok := m.tools.Resolve(toolID)
			if !ok {
				return ToolCommand{}, "", fmt.Errorf("unknown tool id: %s", toolID)
			}
			driver = tasks.WorkerType(profile.Driver)
			extraArgs = append([]string{}, profile.Args...)
			switch driver {
			case tasks.WorkerClaudeCode:
				if strings.TrimSpace(profile.Command) != "" {
					cfg.Paths.Claude = strings.TrimSpace(profile.Command)
				}
			case tasks.WorkerCodex:
				if strings.TrimSpace(profile.Command) != "" {
					cfg.Paths.Codex = strings.TrimSpace(profile.Command)
				}
			case tasks.WorkerExec:
				// handled below
			default:
				return ToolCommand{}, "", ErrUnsupportedWorkerType{WorkerType: driver}
			}
		}
	}

	if driver == tasks.WorkerExec && task.WorkerType != tasks.WorkerExec && m != nil && m.tools != nil {
		toolID := strings.TrimSpace(string(task.WorkerType))
		profile, ok := m.tools.Resolve(toolID)
		if !ok {
			return ToolCommand{}, "", fmt.Errorf("unknown tool id: %s", toolID)
		}
		if task.Mode == tasks.ModeResume {
			return ToolCommand{}, "", fmt.Errorf("tool %q does not support resume mode", toolID)
		}
		return ToolCommand{
			Command: profile.Command,
			Args:    append([]string{}, profile.Args...),
			Dir:     filepath.Clean(task.WorkDir),
			Stdin:   task.Prompt,
		}, driver, nil
	}

	inner := task
	inner.WorkerType = driver
	tool, err := BuildToolCommand(cfg, inner)
	if err != nil {
		return ToolCommand{}, "", err
	}
	if driver == tasks.WorkerCodex {
		tool.Args = m.withCodexDefaults(tool.Args)
	}
	if len(extraArgs) > 0 && (driver == tasks.WorkerClaudeCode || driver == tasks.WorkerCodex) {
		tool.Args = insertArgsBeforeStdinMarker(tool.Args, extraArgs)
	}

	if m != nil && m.store != nil && task.Mode == tasks.ModeNew && (driver == tasks.WorkerClaudeCode || driver == tasks.WorkerCodex) {
		pc, ok, err := m.store.GetProjectContext(ctx)
		if err == nil && ok {
			c, _ := tasks.CompressProjectContext(pc.Content, tasks.MaxProjectContextRunesForWorker)
			if strings.TrimSpace(c) != "" && strings.TrimSpace(tool.Stdin) != "" {
				tool.Stdin = "Project Context:\n" + c + "\n\n---\n\n" + tool.Stdin
			}
		}
	}
	return tool, driver, nil
}

func insertArgsBeforeStdinMarker(args []string, extra []string) []string {
	if len(extra) == 0 {
		return args
	}
	if len(args) == 0 {
		return append([]string{}, extra...)
	}
	if args[len(args)-1] == "-" {
		out := make([]string, 0, len(args)+len(extra))
		out = append(out, args[:len(args)-1]...)
		out = append(out, extra...)
		out = append(out, "-")
		return out
	}
	return append(args, extra...)
}

func (m *Manager) withCodexDefaults(args []string) []string {
	if len(args) == 0 {
		return args
	}
	cmdIdx := -1
	for i, a := range args {
		if a == "e" || a == "exec" || a == "app-server" {
			cmdIdx = i
			break
		}
	}
	if cmdIdx < 0 {
		return args
	}

	model := "gpt-5.2"
	effort := "xhigh"
	if m != nil && m.auth != nil {
		secrets := m.auth.Get()
		if strings.TrimSpace(secrets.CodexModel) != "" {
			model = strings.TrimSpace(secrets.CodexModel)
		}
		if strings.TrimSpace(secrets.CodexReasoningEffort) != "" {
			effort = strings.TrimSpace(secrets.CodexReasoningEffort)
		}
	}

	insert := make([]string, 0, 4)
	if !hasAnyFlag(args, "-m", "--model") && strings.TrimSpace(model) != "" {
		insert = append(insert, "-m", strings.TrimSpace(model))
	}
	if !hasConfigKey(args, "model_reasoning_effort") && strings.TrimSpace(effort) != "" {
		insert = append(insert, "-c", fmt.Sprintf("model_reasoning_effort=%q", strings.TrimSpace(effort)))
	}
	if len(insert) == 0 {
		return args
	}
	out := make([]string, 0, len(args)+len(insert))
	out = append(out, args[:cmdIdx]...)
	out = append(out, insert...)
	out = append(out, args[cmdIdx:]...)
	return out
}

func hasAnyFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func hasConfigKey(args []string, key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c" || args[i] == "--config" {
			v := strings.TrimSpace(args[i+1])
			if strings.HasPrefix(v, key+"=") {
				return true
			}
		}
	}
	return false
}

func isExecutableNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.Error
	if errors.As(err, &ee) && errors.Is(ee.Err, exec.ErrNotFound) {
		return true
	}
	// Best-effort fallback for platform-specific messages.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file or directory")
}

func missingExecutableHint(cmd string, workerType tasks.WorkerType) string {
	name := strings.TrimSpace(cmd)
	if name == "" {
		name = "<empty>"
	}
	switch workerType {
	case tasks.WorkerClaudeCode:
		return fmt.Sprintf(
			`hint: %q not found. If you installed Claude Code as a binary, try: export PATH="$HOME/.local/bin:$PATH". If installed via node/brew, ensure its bin dir is on PATH. Or set an absolute path via config.yaml (paths.claude) or server flag --claude-path.`,
			name,
		)
	case tasks.WorkerCodex:
		return fmt.Sprintf(
			`hint: %q not found. Ensure codex is on PATH, or set an absolute path via config.yaml (paths.codex) or server flag --codex-path.`,
			name,
		)
	default:
		return fmt.Sprintf(`hint: %q not found on PATH.`, name)
	}
}

func (m *Manager) envForWorker(workerType tasks.WorkerType) []string {
	env, _ := m.envForWorkerWithReport(workerType)
	return env
}

func (m *Manager) envForToolWithReport(toolID tasks.WorkerType, driver tasks.WorkerType) ([]string, []string) {
	env, applied := m.envForWorkerWithReport(driver)
	if m == nil || m.tools == nil {
		return env, applied
	}
	id := strings.TrimSpace(string(toolID))
	if id == "" {
		return env, applied
	}
	profile, ok := m.tools.Resolve(id)
	if !ok || len(profile.Env) == 0 {
		return env, applied
	}
	env2, extra := mergeEnvWithReport(env, profile.Env)
	applied = append(applied, extra...)
	return env2, applied
}

func (m *Manager) envForWorkerWithReport(workerType tasks.WorkerType) ([]string, []string) {
	base := os.Environ()
	if m == nil || m.auth == nil {
		return base, nil
	}
	secrets := m.auth.Get()

	additions := map[string]string{}
	switch workerType {
	case tasks.WorkerClaudeCode:
		if strings.TrimSpace(secrets.AnthropicBaseURL) != "" {
			additions["ANTHROPIC_BASE_URL"] = strings.TrimSpace(secrets.AnthropicBaseURL)
		}
		if strings.TrimSpace(secrets.AnthropicAPIKey) != "" {
			additions["ANTHROPIC_API_KEY"] = strings.TrimSpace(secrets.AnthropicAPIKey)
		}
		if strings.TrimSpace(secrets.AnthropicAuthToken) != "" {
			additions["ANTHROPIC_AUTH_TOKEN"] = strings.TrimSpace(secrets.AnthropicAuthToken)
		}
		if strings.TrimSpace(secrets.AnthropicModel) != "" {
			additions["ANTHROPIC_MODEL"] = strings.TrimSpace(secrets.AnthropicModel)
		}
		if strings.TrimSpace(secrets.AnthropicSmallFastModel) != "" {
			additions["ANTHROPIC_SMALL_FAST_MODEL"] = strings.TrimSpace(secrets.AnthropicSmallFastModel)
		}
	case tasks.WorkerCodex:
		if strings.TrimSpace(secrets.OpenAIAPIKey) != "" {
			additions["OPENAI_API_KEY"] = strings.TrimSpace(secrets.OpenAIAPIKey)
		}
	default:
		return base, nil
	}
	out, applied := mergeEnvWithReport(base, additions)
	// Best-effort PATH augmentation for GUI-launched servers (missing shell init).
	if workerType == tasks.WorkerClaudeCode || workerType == tasks.WorkerCodex {
		out, _ = execenv.PrependPATH(out, execenv.DefaultExtraPathDirs())
	}
	return out, applied
}

func mergeEnv(base []string, additions map[string]string) []string {
	out, _ := mergeEnvWithReport(base, additions)
	return out
}

func mergeEnvWithReport(base []string, additions map[string]string) ([]string, []string) {
	out := append([]string{}, base...)

	index := make(map[string]int, len(out))
	valueEmpty := make(map[string]bool, len(out))
	applied := make([]string, 0, len(additions))

	for i, kv := range out {
		j := strings.IndexByte(kv, '=')
		if j <= 0 {
			continue
		}
		k := kv[:j]
		v := kv[j+1:]
		if runtime.GOOS == "windows" {
			k = strings.ToUpper(k)
		}
		if _, ok := index[k]; ok {
			continue
		}
		index[k] = i
		valueEmpty[k] = strings.TrimSpace(v) == ""
	}

	for k, v := range additions {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		check := k
		if runtime.GOOS == "windows" {
			check = strings.ToUpper(check)
		}
		if i, ok := index[check]; ok {
			out[i] = k + "=" + v
			valueEmpty[check] = false
			applied = append(applied, k)
			continue
		}
		index[check] = len(out)
		valueEmpty[check] = false
		applied = append(applied, k)
		out = append(out, k+"="+v)
	}

	sort.Strings(applied)
	return out, applied
}

func (m *Manager) consumeStdout(ctx context.Context, task tasks.Task, driver tasks.WorkerType, stdout io.Reader, claudePeer *claudeProtocolPeer, codexPeer *codexAppServerPeer, runWorkspaceActive bool, initProject bool, sidMu *sync.Mutex, sid *string, cancel context.CancelFunc, resumeFailure *resumeFailureState, blocked *blockedState, toolErrors *toolErrorState) {
	reader := newLineReader(stdout)
	toolUseNames := map[string]string{}
	for {
		line, tooLong, err := readLineWithLimit(reader, defaultJSONLineMaxBytes)
		if err != nil {
			if isEOF(err) {
				return
			}
			m.appendLog(task.ID, tasks.LogSystem, formatReadError(err).Error())
			return
		}
		if tooLong {
			m.appendLog(task.ID, tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}

		// Persist raw stdout for debugging/details (UI can filter by stream).
		m.appendLog(task.ID, tasks.LogStdout, string(line))

		m.handleResumeNotFound(task, driver, string(line), cancel, resumeFailure)

		if claudePeer == nil {
			m.handleApprovalRequired(task, driver, string(line), line, blocked)
		} else if driver == tasks.WorkerClaudeCode {
			m.handleClaudeControlRequest(ctx, task, line, claudePeer, runWorkspaceActive, initProject, blocked)
		}
		if driver == tasks.WorkerCodex && codexPeer != nil {
			codexPeer.HandleLine(line, func(id json.RawMessage, method string, params json.RawMessage) {
				go m.handleCodexServerRequest(ctx, task, codexPeer, id, method, params, runWorkspaceActive, initProject, blocked)
			}, nil)
		}

		var parsed parsedLine
		switch driver {
		case tasks.WorkerClaudeCode:
			parsed, err = parseClaudeJSONLine(line)
		case tasks.WorkerCodex:
			parsed, err = parseCodexJSONLine(line)
		default:
			parsed, err = parsedLine{}, nil
		}

		if err == nil {
			if driver == tasks.WorkerClaudeCode {
				for _, u := range parsed.ToolUses {
					id := strings.TrimSpace(u.ID)
					if id == "" {
						continue
					}
					toolUseNames[id] = strings.TrimSpace(u.Name)
				}
				for _, r := range parsed.ToolResults {
					if !r.IsError {
						continue
					}
					toolErrors.mark()

					id := strings.TrimSpace(r.ToolUseID)
					toolName := strings.TrimSpace(toolUseNames[id])
					if toolName == "" {
						toolName = id
					}
					if toolName == "" {
						toolName = "unknown"
					}

					exitPart := "exit=?"
					if code, ok := parseToolResultExitCode(r.Content); ok {
						exitPart = fmt.Sprintf("exit=%d", code)
					}

					summary := summarizeToolResultContent(r.Content, 500)
					msg := strings.TrimSpace(fmt.Sprintf("tool_error: %s %s %s", toolName, exitPart, summary))
					m.appendLog(task.ID, tasks.LogStderr, msg)
				}
			}
			if parsed.SessionID != "" {
				shouldPublish := false
				sidMu.Lock()
				if strings.TrimSpace(*sid) == "" {
					*sid = parsed.SessionID
					shouldPublish = true
				}
				sidMu.Unlock()
				_ = m.store.SetSessionID(context.Background(), task.ID, parsed.SessionID)
				if shouldPublish {
					m.publishTaskUpdatedForce(task.ID)
				}
			}
			if parsed.AssistantText != "" {
				m.appendLog(task.ID, tasks.LogAssistant, parsed.AssistantText)
			}
			if driver == tasks.WorkerClaudeCode && parsed.IsResult && claudePeer != nil {
				_ = claudePeer.CloseStdin()
			}
		}
	}
}

func isApprovalRequiredLine(line []byte) bool {
	s := strings.ToLower(string(line))
	if strings.Contains(s, "requires approval") {
		return true
	}
	// Claude Code sometimes emits a "requested permissions" message instead of "requires approval".
	// Example: "Claude requested permissions to write to index.html, but you haven't granted it yet."
	if strings.Contains(s, "requested permissions") || strings.Contains(s, "requested permission") {
		return true
	}
	return false
}

const approvalRequiredBlockedReason = "阻塞：需要授权（requires approval），非交互运行无法继续。下一步：开启 workers.unsafe_automation（危险）并重试，或实现审批工作流。"

type resumeFailureState struct {
	mu      sync.Mutex
	seen    bool
	message string
}

func (s *resumeFailureState) setOnce(message string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen {
		return false
	}
	s.seen = true
	s.message = strings.TrimSpace(message)
	return true
}

func (s *resumeFailureState) get() (bool, string) {
	if s == nil {
		return false, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen, s.message
}

type blockedState struct {
	mu     sync.Mutex
	seen   bool
	reason string
}

func (s *blockedState) setOnce(reason string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen {
		return false
	}
	s.seen = true
	s.reason = strings.TrimSpace(reason)
	return true
}

func (s *blockedState) get() (bool, string) {
	if s == nil {
		return false, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen, s.reason
}

func (m *Manager) handleApprovalRequired(task tasks.Task, driver tasks.WorkerType, message string, raw []byte, blocked *blockedState) {
	if driver != tasks.WorkerClaudeCode {
		return
	}
	if raw != nil {
		if !isApprovalRequiredLine(raw) {
			return
		}
	} else {
		if !isApprovalRequiredLine([]byte(message)) {
			return
		}
	}

	if blocked != nil && blocked.setOnce(approvalRequiredBlockedReason) {
		// Best-effort persist for UI; final run status is handled in run() even if these writes fail.
		_ = m.store.SetBlocked(context.Background(), task.ID)
		_ = m.store.SetWarning(context.Background(), task.ID, approvalRequiredBlockedReason)
		m.appendLog(task.ID, tasks.LogSystem, approvalRequiredBlockedReason)
		m.publishTaskUpdatedForce(task.ID)
	}
}

type approvalDecisionOutcome struct {
	Decision  string
	Reason    string
	TimedOut  bool
	Cancelled bool
}

func (m *Manager) waitForApprovalDecision(ctx context.Context, taskID string, approvalID string) approvalDecisionOutcome {
	if m == nil {
		return approvalDecisionOutcome{Decision: "deny", Reason: "approval manager unavailable"}
	}
	taskID = strings.TrimSpace(taskID)
	approvalID = strings.TrimSpace(approvalID)
	if taskID == "" || approvalID == "" {
		return approvalDecisionOutcome{Decision: "deny", Reason: "invalid approval request"}
	}

	timeout := m.approvalTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ch := make(chan approvalDecision, 1)
	m.approvalMu.Lock()
	if m.approvalWaiters == nil {
		m.approvalWaiters = make(map[string]approvalWaiter)
	}
	m.approvalWaiters[approvalID] = approvalWaiter{TaskID: taskID, C: ch}
	m.approvalMu.Unlock()

	defer func() {
		m.approvalMu.Lock()
		if w, ok := m.approvalWaiters[approvalID]; ok && w.C == ch {
			delete(m.approvalWaiters, approvalID)
		}
		m.approvalMu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case dec := <-ch:
		out := approvalDecisionOutcome{
			Decision: strings.ToLower(strings.TrimSpace(dec.Decision)),
			Reason:   strings.TrimSpace(dec.Reason),
		}
		if out.Decision != "approve" && out.Decision != "deny" {
			out.Decision = "deny"
		}
		return out
	case <-timer.C:
		return approvalDecisionOutcome{
			Decision: "deny",
			Reason:   "Approval timed out",
			TimedOut: true,
		}
	case <-ctx.Done():
		return approvalDecisionOutcome{
			Decision:  "deny",
			Reason:    "Approval cancelled",
			Cancelled: true,
		}
	}
}

func (m *Manager) handleClaudeControlRequest(ctx context.Context, task tasks.Task, line []byte, peer *claudeProtocolPeer, runWorkspaceActive bool, initProject bool, blocked *blockedState) {
	if m == nil || peer == nil {
		return
	}
	var env claudeControlRequestEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	if strings.TrimSpace(env.Type) != "control_request" {
		return
	}
	requestID := strings.TrimSpace(env.RequestID)
	if requestID == "" {
		return
	}

	switch strings.TrimSpace(env.Request.Subtype) {
	case "can_use_tool":
		toolName := strings.TrimSpace(env.Request.ToolName)
		result, err := m.onClaudeCanUseTool(ctx, task, toolName, env.Request.Input, env.Request.PermissionSuggestions, env.Request.ToolUseID, runWorkspaceActive, initProject, blocked)
		if err != nil {
			_ = peer.SendControlResponseError(requestID, err.Error())
			return
		}
		_ = peer.SendControlResponseSuccess(requestID, result)
	case "hook_callback":
		// No hooks configured yet; acknowledge to avoid deadlocks.
		_ = peer.SendControlResponseSuccess(requestID, map[string]any{})
	default:
		// Unknown control requests should not block execution.
		_ = peer.SendControlResponseSuccess(requestID, map[string]any{})
	}
}

func claudeRiskLevelForTool(toolName string) tasks.RiskLevel {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read", "glob", "grep", "notebookread":
		return tasks.RiskLow
	case "webfetch", "websearch":
		return tasks.RiskMedium
	default:
		return tasks.RiskHigh
	}
}

func summarizeClaudeToolInput(toolName string, input json.RawMessage) string {
	toolName = strings.TrimSpace(toolName)
	if len(input) == 0 {
		return toolName
	}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return toolName
	}
	getStr := func(k string) string {
		v, ok := obj[k]
		if !ok {
			return ""
		}
		s, ok := v.(string)
		if !ok {
			return ""
		}
		return strings.TrimSpace(s)
	}

	switch strings.ToLower(toolName) {
	case "bash":
		if s := getStr("command"); s != "" {
			return s
		}
		if s := getStr("description"); s != "" {
			return s
		}
	case "webfetch":
		if s := getStr("url"); s != "" {
			return s
		}
		if s := getStr("domain"); s != "" {
			return s
		}
	case "websearch":
		if s := getStr("query"); s != "" {
			return s
		}
		if s := getStr("q"); s != "" {
			return s
		}
	case "write", "edit":
		if s := getStr("file_path"); s != "" {
			return s
		}
		if s := getStr("path"); s != "" {
			return s
		}
	}
	return toolName
}

func (m *Manager) onClaudeCanUseTool(ctx context.Context, task tasks.Task, toolName string, input json.RawMessage, suggestions json.RawMessage, toolUseID string, runWorkspaceActive bool, initProject bool, blocked *blockedState) (claudePermissionResult, error) {
	if m == nil || m.store == nil {
		return claudePermissionResult{}, errors.New("store not configured")
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "Unknown"
	}
	if len(input) == 0 {
		input = []byte("{}")
	}

	lowerTool := strings.ToLower(strings.TrimSpace(toolName))
	if (lowerTool == "write" || lowerTool == "edit") &&
		!runWorkspaceActive &&
		!initProject &&
		strings.ToLower(strings.TrimSpace(task.WorkDirStrategy)) != "worktree" {
		summary := strings.TrimSpace(summarizeClaudeToolInput(toolName, input))
		detail := ""
		if summary != "" {
			detail = "（" + summary + "）"
		}
		reason := strings.TrimSpace(workspaceRequiredWarningPrefix + " 写入被拦截" + detail + "：当前 run 未启用 run workspace。请点击「创建 worktree/workspace 并继续」。")
		if blocked != nil {
			_ = blocked.setOnce(reason)
		}
		_ = m.store.SetBlocked(context.Background(), task.ID)
		_ = m.store.SetWarning(context.Background(), task.ID, reason)
		m.appendLog(task.ID, tasks.LogSystem, reason)
		m.publishTaskUpdatedForce(task.ID)

		interrupt := true
		return claudePermissionResult{
			Behavior:  "deny",
			Message:   reason,
			Interrupt: &interrupt,
		}, nil
	}

	// Auto-approve tools that the task's safety settings already allow (e.g. WebSearch/WebFetch
	// for search-browse intent). This avoids blocking non-interactive runs on tools that were
	// explicitly permitted via --settings.
	if isToolAutoAllowed(task, toolName, input) {
		m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("auto-approved (settings allow): %s", toolName))
		return claudePermissionResult{
			Behavior:     "allow",
			UpdatedInput: input,
		}, nil
	}

	type rawApproval struct {
		ToolName              string          `json:"tool_name"`
		ToolUseID             string          `json:"tool_use_id,omitempty"`
		Input                 json.RawMessage `json:"input"`
		PermissionSuggestions json.RawMessage `json:"permission_suggestions,omitempty"`
	}
	raw := rawApproval{
		ToolName:  toolName,
		ToolUseID: strings.TrimSpace(toolUseID),
		Input:     input,
	}
	if s := strings.TrimSpace(string(suggestions)); s != "" && s != "null" {
		raw.PermissionSuggestions = suggestions
	}
	rawJSON, _ := json.Marshal(raw)

	ar, err := m.store.CreateApprovalRequest(context.Background(), tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: tasks.WorkerClaudeCode,
		WorkDir:    task.WorkDir,
		ActionType: toolName,
		RiskLevel:  claudeRiskLevelForTool(toolName),
		Summary:    summarizeClaudeToolInput(toolName, input),
		Raw:        rawJSON,
	})
	if err != nil {
		return claudePermissionResult{}, err
	}
	_ = m.store.SetAwaitingApproval(context.Background(), task.ID)
	m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("awaiting approval: %s", toolName))
	m.publishTaskUpdatedForce(task.ID)

	outcome := m.waitForApprovalDecision(ctx, task.ID, ar.ID)
	if outcome.TimedOut || outcome.Cancelled {
		_ = m.store.UpdateApprovalRequestDecision(context.Background(), ar.ID, tasks.UpdateApprovalRequestDecisionInput{
			Status: tasks.ApprovalStatusExpired,
			Reason: outcome.Reason,
		})
	}

	_ = m.store.SetRunningStatus(context.Background(), task.ID)
	m.publishTaskUpdatedForce(task.ID)

	m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("approval decided: tool=%s decision=%s", toolName, outcome.Decision))

	if outcome.Decision == "approve" {
		return claudePermissionResult{
			Behavior:     "allow",
			UpdatedInput: input,
		}, nil
	}
	msg := strings.TrimSpace(outcome.Reason)
	if msg == "" {
		msg = "Denied"
	}
	interrupt := false
	return claudePermissionResult{
		Behavior:  "deny",
		Message:   msg,
		Interrupt: &interrupt,
	}, nil
}

func isApprovalBlockedReason(observed string, existingWarning string) bool {
	observed = strings.ToLower(strings.TrimSpace(observed))
	existingWarning = strings.ToLower(strings.TrimSpace(existingWarning))
	if strings.Contains(observed, "requires approval") {
		return true
	}
	if strings.Contains(existingWarning, "requires approval") {
		return true
	}
	return false
}

func (m *Manager) handleResumeNotFound(task tasks.Task, driver tasks.WorkerType, message string, cancel context.CancelFunc, resumeFailure *resumeFailureState) {
	if task.Mode != tasks.ModeResume || driver != tasks.WorkerClaudeCode {
		return
	}
	if !isResumeConversationNotFound(message) {
		return
	}
	msg := extractResumeNotFoundMessage(message)
	if msg == "" {
		msg = "No conversation found for requested session."
	}
	if resumeFailure.setOnce(msg) {
		m.appendLog(task.ID, tasks.LogSystem, "resume failed: "+msg)
		_ = m.store.SetWarning(context.Background(), task.ID, "resume failed: "+msg)
		if cancel != nil {
			cancel()
		}
	}
}

func isResumeConversationNotFound(message string) bool {
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "no conversation found") {
		return false
	}
	return strings.Contains(lower, "session")
}

func extractResumeNotFoundMessage(message string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	idx := strings.Index(lower, "no conversation found")
	if idx < 0 {
		return msg
	}
	return strings.TrimSpace(msg[idx:])
}

func (m *Manager) consumeLines(task tasks.Task, driver tasks.WorkerType, stream tasks.LogStream, r io.Reader, cancel context.CancelFunc, resumeFailure *resumeFailureState, blocked *blockedState) {
	reader := newLineReader(r)
	for {
		line, tooLong, err := readLineWithLimit(reader, 1024*1024)
		if err != nil {
			if isEOF(err) {
				return
			}
			m.appendLog(task.ID, tasks.LogSystem, formatReadError(err).Error())
			return
		}
		if tooLong {
			m.appendLog(task.ID, tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}
		msg := string(line)
		m.appendLog(task.ID, stream, msg)
		m.handleResumeNotFound(task, driver, msg, cancel, resumeFailure)
		m.handleApprovalRequired(task, driver, msg, nil, blocked)
	}
}

func (m *Manager) publishTaskUpdated(taskID string) {
	if m.hub == nil {
		return
	}
	task, err := m.store.GetTask(context.Background(), taskID)
	if err != nil {
		return
	}
	m.hub.Publish(events.Event{
		Type:    "task.updated",
		Time:    time.Now().UTC(),
		Payload: task,
	})
}

const taskUpdateThrottle = 500 * time.Millisecond

func (m *Manager) publishTaskUpdatedThrottled(taskID string) {
	if m == nil || m.hub == nil || m.store == nil {
		return
	}
	now := time.Now().UTC()

	m.updateMu.Lock()
	if m.lastTaskUpdate == nil {
		m.lastTaskUpdate = make(map[string]time.Time)
	}
	last := m.lastTaskUpdate[taskID]
	if !last.IsZero() && now.Sub(last) < taskUpdateThrottle {
		m.updateMu.Unlock()
		return
	}
	m.lastTaskUpdate[taskID] = now
	m.updateMu.Unlock()

	m.publishTaskUpdated(taskID)
}

func (m *Manager) publishTaskUpdatedForce(taskID string) {
	if m == nil || m.hub == nil || m.store == nil {
		return
	}
	now := time.Now().UTC()
	m.updateMu.Lock()
	if m.lastTaskUpdate == nil {
		m.lastTaskUpdate = make(map[string]time.Time)
	}
	m.lastTaskUpdate[taskID] = now
	m.updateMu.Unlock()
	m.publishTaskUpdated(taskID)
}

func (m *Manager) publishLog(entry tasks.LogEntry) {
	if m.hub == nil {
		return
	}
	m.hub.Publish(events.Event{
		Type:    "task.log",
		Time:    time.Now().UTC(),
		Payload: entry,
	})
}

func (m *Manager) appendLog(taskID string, stream tasks.LogStream, message string) {
	if m == nil || m.store == nil {
		return
	}
	entry, err := m.store.AppendLog(context.Background(), taskID, stream, message)
	if err != nil {
		return
	}
	m.publishLog(entry)
	m.publishTaskUpdatedThrottled(taskID)
}

func (m *Manager) failTask(taskID string, err error) error {
	m.appendLog(taskID, tasks.LogSystem, err.Error())
	_ = m.store.FinishTask(context.Background(), taskID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		ExitCode:   nil,
		Error:      err.Error(),
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	})
	m.publishTaskUpdated(taskID)
	return err
}

func formatRunStartLog(toolID tasks.WorkerType, driver tasks.WorkerType, tool ToolCommand, injectedEnvKeys []string) string {
	env := formatQuotedList(injectedEnvKeys)
	driverPart := ""
	if strings.TrimSpace(string(driver)) != "" && driver != toolID {
		driverPart = fmt.Sprintf(" driver=%s", driver)
	}
	return fmt.Sprintf("run.start worker=%s%s dir=%q cmd=%q args=%s env_injected=%s", toolID, driverPart, tool.Dir, tool.Command, formatQuotedList(tool.Args), env)
}

func formatRunFinishLog(status tasks.Status, exitCode *int, errText string) string {
	msg := fmt.Sprintf("run.finish status=%s exit_code=%s", status, formatExitCode(exitCode))
	if strings.TrimSpace(errText) != "" {
		msg += fmt.Sprintf(" err=%q", errText)
	}
	return msg
}

func formatExitCode(exitCode *int) string {
	if exitCode == nil {
		return "null"
	}
	return fmt.Sprintf("%d", *exitCode)
}

func formatQuotedList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, fmt.Sprintf("%q", item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
