package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/events"
	"controlccx/internal/execenv"
	"controlccx/internal/tasks"
)

type Manager struct {
	cfg   config.Config
	store *tasks.Store
	hub   *events.Hub
	auth  *auth.Store

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	updateMu       sync.Mutex
	lastTaskUpdate map[string]time.Time
}

func NewManager(cfg config.Config, store *tasks.Store, hub *events.Hub, authStore *auth.Store) *Manager {
	return &Manager{
		cfg:     cfg,
		store:   store,
		hub:     hub,
		auth:    authStore,
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
	if err := m.store.SetRunning(context.Background(), task.ID); err != nil {
		return err
	}
	m.publishTaskUpdated(task.ID)

	tool, err := m.buildToolCommand(task)
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
	env, injectedEnvKeys := m.envForWorkerWithReport(task.WorkerType)
	cmd.Env = env

	m.appendLog(task.ID, tasks.LogSystem, formatRunStartLog(task.WorkerType, tool, injectedEnvKeys))

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
			m.appendLog(task.ID, tasks.LogSystem, missingExecutableHint(tool.Command, task.WorkerType))
		}
		return m.failTask(task.ID, fmt.Errorf("start: %w", err))
	}

	var (
		lastSessionIDMu sync.Mutex
		lastSessionID   string
	)

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		m.consumeStdout(task, stdout, &lastSessionIDMu, &lastSessionID)
	}()

	go func() {
		defer wg.Done()
		m.consumeLines(task.ID, tasks.LogStderr, stderr)
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	exitCode := exitCode(waitErr)
	status := tasks.StatusSucceeded
	errText := ""
	if errors.Is(ctx.Err(), context.Canceled) {
		status = tasks.StatusCanceled
	} else if waitErr != nil {
		status = tasks.StatusFailed
		errText = waitErr.Error()
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

func (m *Manager) buildToolCommand(task tasks.Task) (ToolCommand, error) {
	tool, err := BuildToolCommand(m.cfg, task)
	if err != nil {
		return ToolCommand{}, err
	}
	if task.WorkerType == tasks.WorkerCodex {
		tool.Args = m.withCodexDefaults(tool.Args)
	}
	return tool, nil
}

func (m *Manager) withCodexDefaults(args []string) []string {
	if len(args) == 0 {
		return args
	}
	if args[0] != "e" && args[0] != "exec" {
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
	out = append(out, args[:1]...)
	out = append(out, insert...)
	out = append(out, args[1:]...)
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

func (m *Manager) consumeStdout(task tasks.Task, stdout io.Reader, sidMu *sync.Mutex, sid *string) {
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

		var parsed parsedLine
		switch task.WorkerType {
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

func (m *Manager) consumeLines(taskID string, stream tasks.LogStream, r io.Reader) {
	reader := newLineReader(r)
	for {
		line, tooLong, err := readLineWithLimit(reader, 1024*1024)
		if err != nil {
			if isEOF(err) {
				return
			}
			m.appendLog(taskID, tasks.LogSystem, formatReadError(err).Error())
			return
		}
		if tooLong {
			m.appendLog(taskID, tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}
		m.appendLog(taskID, stream, string(line))
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

func formatRunStartLog(workerType tasks.WorkerType, tool ToolCommand, injectedEnvKeys []string) string {
	env := formatQuotedList(injectedEnvKeys)
	return fmt.Sprintf("run.start worker=%s dir=%q cmd=%q args=%s env_injected=%s", workerType, tool.Dir, tool.Command, formatQuotedList(tool.Args), env)
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
