package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

type Manager struct {
	cfg   config.Config
	store *tasks.Store
	hub   *events.Hub
	auth  *auth.Store

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
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

		tool, err := BuildToolCommand(m.cfg, task)
		if err != nil {
			_, _ = m.store.AppendLog(context.Background(), task.ID, tasks.LogSystem, fmt.Sprintf("worker setup error: %v", err))
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
		_, _ = m.store.AppendLog(context.Background(), task.ID, tasks.LogSystem, tool.Warning)
		m.publishTaskUpdated(task.ID)
	}

		cmd := exec.CommandContext(ctx, tool.Command, tool.Args...)
		cmd.Dir = tool.Dir
		cmd.Env = m.envForWorker(task.WorkerType)

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

	lastSessionIDMu.Lock()
	sid := lastSessionID
	lastSessionIDMu.Unlock()

	_ = m.store.FinishTask(context.Background(), task.ID, tasks.FinishTaskInput{
		Status:     status,
		ExitCode:   exitCode,
		Error:      errText,
		SessionID:  sid,
		FinishedAt: time.Now().UTC(),
	})
	m.publishTaskUpdated(task.ID)
	return nil
}

func (m *Manager) envForWorker(workerType tasks.WorkerType) []string {
	base := os.Environ()
	if m == nil || m.auth == nil {
		return base
	}
	secrets := m.auth.Get()

	additions := map[string]string{}
	switch workerType {
	case tasks.WorkerClaudeCode:
		if strings.TrimSpace(secrets.AnthropicAPIKey) != "" {
			additions["ANTHROPIC_API_KEY"] = strings.TrimSpace(secrets.AnthropicAPIKey)
		}
		if strings.TrimSpace(secrets.AnthropicAuthToken) != "" {
			additions["ANTHROPIC_AUTH_TOKEN"] = strings.TrimSpace(secrets.AnthropicAuthToken)
		}
	case tasks.WorkerCodex:
		if strings.TrimSpace(secrets.OpenAIAPIKey) != "" {
			additions["OPENAI_API_KEY"] = strings.TrimSpace(secrets.OpenAIAPIKey)
		}
	default:
		return base
	}
	return mergeEnv(base, additions)
}

func mergeEnv(base []string, additions map[string]string) []string {
	out := append([]string{}, base...)

	index := make(map[string]int, len(out))
	valueEmpty := make(map[string]bool, len(out))

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
			}
			continue
		}
		index[check] = len(out)
		valueEmpty[check] = false
		out = append(out, k+"="+v)
	}

	return out
}

func (m *Manager) consumeStdout(task tasks.Task, stdout io.Reader, sidMu *sync.Mutex, sid *string) {
	reader := newLineReader(stdout)
	for {
		line, tooLong, err := readLineWithLimit(reader, defaultJSONLineMaxBytes)
		if err != nil {
			if isEOF(err) {
				return
			}
			_, _ = m.store.AppendLog(context.Background(), task.ID, tasks.LogSystem, formatReadError(err).Error())
			return
		}
		if tooLong {
			_, _ = m.store.AppendLog(context.Background(), task.ID, tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}

		// Always persist raw stdout.
		logEntry, _ := m.store.AppendLog(context.Background(), task.ID, tasks.LogStdout, string(line))
		m.publishLog(logEntry)

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
				_ = m.store.SetSessionID(context.Background(), task.ID, parsed.SessionID)
				sidMu.Lock()
				*sid = parsed.SessionID
				sidMu.Unlock()
			}
			if parsed.AssistantText != "" {
				assistantEntry, _ := m.store.AppendLog(context.Background(), task.ID, tasks.LogAssistant, parsed.AssistantText)
				m.publishLog(assistantEntry)
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
			_, _ = m.store.AppendLog(context.Background(), taskID, tasks.LogSystem, formatReadError(err).Error())
			return
		}
		if tooLong {
			_, _ = m.store.AppendLog(context.Background(), taskID, tasks.LogSystem, "skipped overlong output line")
			continue
		}
		if len(line) == 0 {
			continue
		}
		entry, _ := m.store.AppendLog(context.Background(), taskID, stream, string(line))
		m.publishLog(entry)
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

func (m *Manager) failTask(taskID string, err error) error {
	_, _ = m.store.AppendLog(context.Background(), taskID, tasks.LogSystem, err.Error())
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
