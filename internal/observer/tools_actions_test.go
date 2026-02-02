package observer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

type fakeRunner struct {
	started  []string
	canceled []string
	startErr error
	cancelOK bool
}

func (r *fakeRunner) Start(ctx context.Context, taskID string) error {
	r.started = append(r.started, taskID)
	return r.startErr
}

func (r *fakeRunner) Cancel(taskID string) bool {
	r.canceled = append(r.canceled, taskID)
	return r.cancelOK
}

func TestTools_taskResume_CreatesAndStartsResumeRun(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	res, err := tools["task_resume"].Run(ctx, map[string]any{
		"task_id": prev.ID,
		"prompt":  "继续",
	})
	if err != nil {
		t.Fatalf("tool run: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result")
	}
	if len(r.started) != 1 {
		t.Fatalf("started=%v, want 1 start", r.started)
	}

	next, err := store.GetTask(ctx, r.started[0])
	if err != nil {
		t.Fatalf("get resumed task: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q want %q", next.Mode, tasks.ModeResume)
	}
	if strings.TrimSpace(next.SessionID) != "sess-1" {
		t.Fatalf("session_id=%q want %q", next.SessionID, "sess-1")
	}
	if next.Prompt != "继续" {
		t.Fatalf("prompt=%q want %q", next.Prompt, "继续")
	}
	if next.WorkDir != "." {
		t.Fatalf("workdir=%q want %q", next.WorkDir, ".")
	}
}

func TestTools_taskResume_RejectsOverlappingRunsInSession(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   nil,
		Error:      "",
		SessionID:  "",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	running, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "running",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create running: %v", err)
	}
	if err := store.SetRunning(ctx, running.ID); err != nil {
		t.Fatalf("set running: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	_, err = tools["task_resume"].Run(ctx, map[string]any{
		"task_id": prev.ID,
		"prompt":  "continue",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already has a running task") {
		t.Fatalf("err=%q, want contains %q", err.Error(), "already has a running task")
	}
	if len(r.started) != 0 {
		t.Fatalf("started=%v, want 0", r.started)
	}
}

func TestTools_sessionContinue_CreatesResumeOrRehydrateRun(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	first, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "do A",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, _ = store.AppendLog(ctx, first.ID, tasks.LogAssistant, "done A")
	if err := store.FinishTask(ctx, first.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish first: %v", err)
	}

	time.Sleep(2 * time.Millisecond)
	resume, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeResume,
		ConversationID: first.ConversationID,
		Prompt:         "continue",
		WorkDir:        ".",
		SessionID:      "sess-1",
	})
	if err != nil {
		t.Fatalf("create resume: %v", err)
	}
	if err := store.SetWarning(ctx, resume.ID, "resume failed: No conversation found with session ID: sess-1"); err != nil {
		t.Fatalf("set warning: %v", err)
	}
	if err := store.FinishTask(ctx, resume.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish resume: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	_, err = tools["session_continue"].Run(ctx, map[string]any{
		"task_id": first.ID,
		"prompt":  "继续",
	})
	if err != nil {
		t.Fatalf("tool run: %v", err)
	}
	if len(r.started) != 1 {
		t.Fatalf("started=%v, want 1 start", r.started)
	}
	next, err := store.GetTask(ctx, r.started[0])
	if err != nil {
		t.Fatalf("get continued task: %v", err)
	}
	if next.Mode != tasks.ModeNew {
		t.Fatalf("mode=%q want %q", next.Mode, tasks.ModeNew)
	}
	if next.ConversationID != first.ConversationID {
		t.Fatalf("conversation_id=%q want %q", next.ConversationID, first.ConversationID)
	}
	if !strings.Contains(next.Prompt, "do A") || !strings.Contains(next.Prompt, "done A") {
		t.Fatalf("rehydrate prompt missing context: %q", next.Prompt)
	}
	if !strings.Contains(next.Prompt, "[controlccx rehydrate]") {
		t.Fatalf("prompt missing header: %q", next.Prompt)
	}
}

func TestTools_sessionContinue_CreatesResumeRunWhenHealthy(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	prev, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := store.FinishTask(ctx, prev.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		SessionID:  "sess-1",
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish prev: %v", err)
	}

	r := &fakeRunner{cancelOK: true}
	svc := &Service{Store: store, Runner: r}
	tools := svc.agentTools()

	_, err = tools["session_continue"].Run(ctx, map[string]any{
		"task_id": prev.ID,
		"prompt":  "继续",
	})
	if err != nil {
		t.Fatalf("tool run: %v", err)
	}
	if len(r.started) != 1 {
		t.Fatalf("started=%v, want 1 start", r.started)
	}

	next, err := store.GetTask(ctx, r.started[0])
	if err != nil {
		t.Fatalf("get resumed task: %v", err)
	}
	if next.Mode != tasks.ModeResume {
		t.Fatalf("mode=%q want %q", next.Mode, tasks.ModeResume)
	}
	if strings.TrimSpace(next.SessionID) != "sess-1" {
		t.Fatalf("session_id=%q want %q", next.SessionID, "sess-1")
	}
}

func TestTools_acceptanceUpdate_UpsertsAndMerges(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "hi",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	key := tasks.SessionKeyForTask(task)

	svc := &Service{Store: store}
	tools := svc.agentTools()

	_, err = tools["acceptance_update"].Run(ctx, map[string]any{
		"task_id":        task.ID,
		"status":         "running",
		"iteration":      1,
		"max_iterations": 10,
		"current_gate":   "runnability.smoke",
		"summary":        "starting",
		"plan_json":      `{"intent":"runnable"}`,
	})
	if err != nil {
		t.Fatalf("acceptance_update: %v", err)
	}

	st, ok, err := store.GetAcceptanceState(ctx, key)
	if err != nil {
		t.Fatalf("get acceptance: %v", err)
	}
	if !ok {
		t.Fatalf("expected state present")
	}
	if st.Iteration != 1 || st.CurrentGate != "runnability.smoke" {
		t.Fatalf("unexpected state=%+v", st)
	}

	// Partial update should merge (keep iteration/gate).
	_, err = tools["acceptance_update"].Run(ctx, map[string]any{
		"key":     key,
		"summary": "still running",
	})
	if err != nil {
		t.Fatalf("acceptance_update partial: %v", err)
	}
	st2, ok, err := store.GetAcceptanceState(ctx, key)
	if err != nil {
		t.Fatalf("get acceptance2: %v", err)
	}
	if !ok {
		t.Fatalf("expected state present")
	}
	if st2.Iteration != 1 || st2.CurrentGate != "runnability.smoke" || st2.Summary != "still running" {
		t.Fatalf("unexpected merged state=%+v", st2)
	}

	res, err := tools["acceptance_get"].Run(ctx, map[string]any{"key": key})
	if err != nil {
		t.Fatalf("acceptance_get: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", res)
	}
	if m["ok"] != true {
		t.Fatalf("expected ok=true, got %v", m["ok"])
	}
}

func TestTools_taskOutputStats_IncludesHeadingCounts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "write something",
		WorkDir:    ".",
		SessionID:  "sess-1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, err = store.AppendLog(ctx, task.ID, tasks.LogAssistant, strings.Join([]string{
		"# Title",
		"## A",
		"### B",
		"1. First",
		"1.1 Second",
		"一、Third",
		"第1章 Intro",
		"",
		"body",
	}, "\n"))
	if err != nil {
		t.Fatalf("append log: %v", err)
	}

	svc := &Service{Store: store}
	tools := svc.agentTools()

	res, err := tools["task_output_stats"].Run(ctx, map[string]any{
		"task_id": task.ID,
	})
	if err != nil {
		t.Fatalf("task_output_stats: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", res)
	}
	stats, ok := m["stats"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected stats type: %T", m["stats"])
	}

	if stats["headings_markdown"] != 3 {
		t.Fatalf("headings_markdown=%v want %v", stats["headings_markdown"], 3)
	}
	if stats["headings_numbered"] != 2 {
		t.Fatalf("headings_numbered=%v want %v", stats["headings_numbered"], 2)
	}
	if stats["headings_chinese"] != 2 {
		t.Fatalf("headings_chinese=%v want %v", stats["headings_chinese"], 2)
	}
	if stats["heading_lines"] != 7 {
		t.Fatalf("heading_lines=%v want %v", stats["heading_lines"], 7)
	}
	if stats["sections"] != 7 {
		t.Fatalf("sections=%v want %v", stats["sections"], 7)
	}
}
