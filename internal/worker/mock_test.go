package worker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

// mockStore implements TaskStore for unit tests without requiring SQLite.
// Each method records calls and returns preset values. Thread-safe via mutex.
type mockStore struct {
	mu sync.Mutex

	// Task state tracking
	tasks       map[string]tasks.Task
	taskStatus  map[string]tasks.Status
	logs        []tasks.LogEntry
	warnings    map[string]string
	sessionIDs  map[string]string
	invocations []mockInvocation
	touchCount  int

	// Approval flow
	approvals      []tasks.ApprovalRequest
	nextApprovalID string

	// Configurable returns
	getTaskErr     error
	setRunningErr  error
	finishTaskErr  error
	appendLogErr   error
	projectContext *tasks.ProjectContext
	dequeueResult  *tasks.Task

	// Call tracking
	finishCalls []mockFinishCall
}

type mockInvocation struct {
	TaskID  string
	Cmd     string
	Args    []string
	Dir     string
	EnvKeys []string
}

type mockFinishCall struct {
	TaskID string
	Input  tasks.FinishTaskInput
}

func newMockStore() *mockStore {
	return &mockStore{
		tasks:      make(map[string]tasks.Task),
		taskStatus: make(map[string]tasks.Status),
		warnings:   make(map[string]string),
		sessionIDs: make(map[string]string),
	}
}

func (s *mockStore) CreateTask(_ context.Context, in tasks.CreateTaskInput) (tasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := tasks.Task{
		ID:         "task-" + in.Prompt[:min(8, len(in.Prompt))],
		WorkerType: in.WorkerType,
		Mode:       in.Mode,
		Prompt:     in.Prompt,
		WorkDir:    in.WorkDir,
		Status:     tasks.StatusQueued,
	}
	s.tasks[t.ID] = t
	s.taskStatus[t.ID] = t.Status
	return t, nil
}

func (s *mockStore) GetTask(_ context.Context, id string) (tasks.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getTaskErr != nil {
		return tasks.Task{}, s.getTaskErr
	}
	t, ok := s.tasks[id]
	if !ok {
		return tasks.Task{}, fmt.Errorf("task not found: %s", id)
	}
	if status, ok := s.taskStatus[id]; ok {
		t.Status = status
	}
	if w, ok := s.warnings[id]; ok {
		t.Warning = w
	}
	if sid, ok := s.sessionIDs[id]; ok {
		t.SessionID = sid
	}
	return t, nil
}

func (s *mockStore) SetRunning(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setRunningErr != nil {
		return s.setRunningErr
	}
	s.taskStatus[id] = tasks.StatusRunning
	return nil
}

func (s *mockStore) SetBlocked(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskStatus[id] = tasks.StatusBlocked
	return nil
}

func (s *mockStore) SetAwaitingApproval(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskStatus[id] = tasks.StatusAwaitingApproval
	return nil
}

func (s *mockStore) SetRunningStatus(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskStatus[id] = tasks.StatusRunning
	return nil
}

func (s *mockStore) FinishTask(_ context.Context, id string, in tasks.FinishTaskInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finishTaskErr != nil {
		return s.finishTaskErr
	}
	s.taskStatus[id] = in.Status
	s.finishCalls = append(s.finishCalls, mockFinishCall{TaskID: id, Input: in})
	return nil
}

func (s *mockStore) TouchTask(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touchCount++
	return nil
}

func (s *mockStore) SetWarning(_ context.Context, id, warning string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings[id] = warning
	return nil
}

func (s *mockStore) SetSessionID(_ context.Context, id, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionIDs[id] = sessionID
	return nil
}

func (s *mockStore) SetInvocation(_ context.Context, taskID, cmd string, args []string, dir string, envKeys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invocations = append(s.invocations, mockInvocation{
		TaskID: taskID, Cmd: cmd, Args: args, Dir: dir, EnvKeys: envKeys,
	})
	return nil
}

func (s *mockStore) AppendLog(_ context.Context, taskID string, stream tasks.LogStream, message string) (tasks.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appendLogErr != nil {
		return tasks.LogEntry{}, s.appendLogErr
	}
	entry := tasks.LogEntry{
		ID:      int64(len(s.logs) + 1),
		TaskID:  taskID,
		Stream:  stream,
		Message: message,
	}
	s.logs = append(s.logs, entry)
	return entry, nil
}

func (s *mockStore) ListLogs(_ context.Context, taskID string, afterID int64, limit int) ([]tasks.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []tasks.LogEntry
	for _, l := range s.logs {
		if l.TaskID == taskID && l.ID > afterID {
			result = append(result, l)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (s *mockStore) CreateApprovalRequest(_ context.Context, in tasks.CreateApprovalRequestInput) (tasks.ApprovalRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ar := tasks.ApprovalRequest{
		ID:     s.nextApprovalID,
		TaskID: in.TaskID,
		Status: tasks.ApprovalStatusPending,
	}
	s.approvals = append(s.approvals, ar)
	return ar, nil
}

func (s *mockStore) UpdateApprovalRequestDecision(_ context.Context, _ string, _ tasks.UpdateApprovalRequestDecisionInput) error {
	return nil
}

func (s *mockStore) DequeueNextWaitingForWorkdir(_ context.Context, _ string) (tasks.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dequeueResult != nil {
		return *s.dequeueResult, true, nil
	}
	return tasks.Task{}, false, nil
}

func (s *mockStore) GetProjectContext(_ context.Context) (tasks.ProjectContext, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projectContext != nil {
		return *s.projectContext, true, nil
	}
	return tasks.ProjectContext{}, false, nil
}

// getFinishCalls returns a snapshot of all FinishTask calls.
func (s *mockStore) getFinishCalls() []mockFinishCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]mockFinishCall, len(s.finishCalls))
	copy(out, s.finishCalls)
	return out
}

// mockHub implements EventPublisher for unit tests.
type mockHub struct {
	mu     sync.Mutex
	events []events.Event
}

func (h *mockHub) Publish(evt events.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, evt)
}

// mockProcessRunner implements ProcessRunner for unit tests.
type mockProcessRunner struct {
	mu       sync.Mutex
	spawnErr error
	runs     []*mockManagedRun
}

func newMockProcessRunner() *mockProcessRunner {
	return &mockProcessRunner{}
}

func (r *mockProcessRunner) Spawn(ctx context.Context, opts SpawnOpts) (ManagedRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.spawnErr != nil {
		return nil, r.spawnErr
	}

	run := &mockManagedRun{
		opts:   opts,
		stdout: new(bytes.Buffer),
		stderr: new(bytes.Buffer),
		stdin:  new(bytes.Buffer),
		waitCh: make(chan struct{}),
	}
	r.runs = append(r.runs, run)

	go func() {
		select {
		case <-ctx.Done():
			_ = run.Cancel()
		case <-run.waitCh:
		}
	}()

	return run, nil
}

// mockManagedRun implements ManagedRun for unit tests.
type mockManagedRun struct {
	mu       sync.Mutex
	opts     SpawnOpts
	pid      int
	stdout   *bytes.Buffer
	stderr   *bytes.Buffer
	stdin    *bytes.Buffer
	exitCode int
	waitErr  error
	waitCh   chan struct{}
	canceled bool
}

func (r *mockManagedRun) PID() int {
	return r.pid
}

func (r *mockManagedRun) Stdout() io.Reader {
	return r.stdout
}

func (r *mockManagedRun) Stderr() io.Reader {
	return r.stderr
}

func (r *mockManagedRun) Stdin() io.WriteCloser {
	return &nopWriteCloser{r.stdin}
}

func (r *mockManagedRun) Wait() (int, error) {
	<-r.waitCh
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exitCode, r.waitErr
}

func (r *mockManagedRun) Cancel() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canceled = true
	select {
	case <-r.waitCh:
	default:
		r.waitErr = context.Canceled
		close(r.waitCh)
	}
	return nil
}

// emulateFinish triggers the simulated process exit.
func (r *mockManagedRun) emulateFinish(exitCode int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-r.waitCh:
		// already canceled/finished
		return
	default:
		r.exitCode = exitCode
		r.waitErr = err
		close(r.waitCh)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
