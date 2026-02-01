package worker

import (
	"context"
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
)

type Manager struct {
	cfg   config.Config
	store *tasks.Store
	hub   *events.Hub
	auth  *auth.Store
	tools *tooling.Service

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	updateMu       sync.Mutex
	lastTaskUpdate map[string]time.Time
}

func NewManager(cfg config.Config, store *tasks.Store, hub *events.Hub, authStore *auth.Store, tools *tooling.Service) *Manager {
	return &Manager{
		cfg:     cfg,
		store:   store,
		hub:     hub,
		auth:    authStore,
		tools:   tools,
		cancels: make(map[string]context.CancelFunc),
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

	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
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

func (m *Manager) Cancel(taskID string) bool {
	m.mu.Lock()
	cancel, ok := m.cancels[taskID]
	m.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (m *Manager) run(ctx context.Context, task tasks.Task) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := m.store.SetRunning(context.Background(), task.ID); err != nil {
		return err
	}
	m.publishTaskUpdated(task.ID)

	tool, driver, err := m.buildToolCommand(task)
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

	if (driver == tasks.WorkerClaudeCode || driver == tasks.WorkerCodex) && m != nil && m.store != nil {
		ws, err := runworkspace.NewService(m.store).EnsureForTask(ctx, task)
		if err != nil {
			m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace warning: %v", err))
		} else if strings.TrimSpace(ws.RunWorkDir) != "" {
			runTask := task
			runTask.WorkDir = ws.RunWorkDir
			if nextTool, nextDriver, err := m.buildToolCommand(runTask); err != nil {
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace warning: failed to rebuild tool command: %v", err))
			} else {
				tool = nextTool
				driver = nextDriver
				m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("workspace.active kind=%s base=%s run=%s", ws.Kind, ws.BaseWorkDir, ws.RunWorkDir))
				tool.Stdin = injectWorkspaceContextPrompt(tool.Stdin, ws)
			}
		}
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
	if tool.Stdin != "" {
		cmd.Stdin = stringsReader(tool.Stdin)
	}

	if err := cmd.Start(); err != nil {
		if isExecutableNotFound(err) {
			m.appendLog(task.ID, tasks.LogSystem, missingExecutableHint(tool.Command, driver))
		}
		return m.failTask(task.ID, fmt.Errorf("start: %w", err))
	}

	var (
		lastSessionIDMu sync.Mutex
		lastSessionID   string
	)
	resumeFailure := &resumeFailureState{}
	blockedState := &blockedState{}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.consumeStdout(task, driver, stdout, &lastSessionIDMu, &lastSessionID, cancel, resumeFailure, blockedState)
	}()

	go func() {
		defer wg.Done()
		m.consumeLines(task, driver, tasks.LogStderr, stderr, cancel, resumeFailure, blockedState)
	}()

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
	m.publishTaskUpdated(task.ID)
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

func injectWorkspaceContextPrompt(prompt string, ws tasks.SessionWorkspace) string {
	if strings.Contains(prompt, "[controlccx workspace]") {
		return prompt
	}
	base := strings.TrimSpace(ws.BaseWorkDir)
	run := strings.TrimSpace(ws.RunWorkDir)
	kind := strings.TrimSpace(string(ws.Kind))
	if base == "" || run == "" || kind == "" {
		return prompt
	}

	note := fmt.Sprintf(
		`[controlccx workspace]
This run uses an isolated workspace (%s).
- base_workdir: %s
- run_workdir: %s
When you mention file paths to the user, prefer base_workdir and clarify when using run_workdir.
向用户输出路径时优先使用 base_workdir；run_workdir 是隔离工作区路径，需要时请说明。
[/controlccx workspace]

`,
		kind,
		base,
		run,
	)
	return note + prompt
}

func (m *Manager) buildToolCommand(task tasks.Task) (ToolCommand, tasks.WorkerType, error) {
	driver := task.WorkerType
	cfg := m.cfg
	extraArgs := []string(nil)

	if m != nil && m.tools != nil {
		toolID := strings.TrimSpace(string(task.WorkerType))
		if toolID != "" {
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

	if driver == tasks.WorkerExec && m != nil && m.tools != nil {
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
	execIdx := -1
	for i, a := range args {
		if a == "e" || a == "exec" {
			execIdx = i
			break
		}
	}
	if execIdx < 0 {
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
	out = append(out, args[:execIdx+1]...)
	out = append(out, insert...)
	out = append(out, args[execIdx+1:]...)
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
			if valueEmpty[check] {
				out[i] = k + "=" + v
				valueEmpty[check] = false
				applied = append(applied, k)
			}
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

func (m *Manager) consumeStdout(task tasks.Task, driver tasks.WorkerType, stdout io.Reader, sidMu *sync.Mutex, sid *string, cancel context.CancelFunc, resumeFailure *resumeFailureState, blocked *blockedState) {
	reader := newLineReader(stdout)
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

		m.handleApprovalRequired(task, driver, string(line), line, blocked)

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

	const reason = "Blocked: requires approval (non-interactive run cannot proceed). Next: enable workers.unsafe_automation (dangerous) and retry, or implement approval workflow."
	if blocked != nil && blocked.setOnce(reason) {
		// Best-effort persist for UI; final run status is handled in run() even if these writes fail.
		_ = m.store.SetBlocked(context.Background(), task.ID)
		_ = m.store.SetWarning(context.Background(), task.ID, reason)
		m.appendLog(task.ID, tasks.LogSystem, reason)
		m.publishTaskUpdatedForce(task.ID)
	}
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
