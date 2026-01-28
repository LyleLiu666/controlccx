package worker

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_consumeStdout_ClaudeCode_StoresRawStdout(t *testing.T) {
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
		Prompt:     "x",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := &Manager{cfg: config.Default(), store: store}

	var (
		sidMu sync.Mutex
		sid   string
	)
	out := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"sess-1"}`,
		`{"type":"assistant","session_id":"sess-1","result":"final answer"}`,
	}, "\n")

	m.consumeStdout(task, tasks.WorkerClaudeCode, strings.NewReader(out), &sidMu, &sid, func() {}, &resumeFailureState{}, &blockedState{})

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}

	stdoutCount := 0
	assistantCount := 0
	for _, l := range logs {
		if l.Stream == tasks.LogStdout {
			stdoutCount++
		}
		if l.Stream == tasks.LogAssistant {
			assistantCount++
		}
	}
	if stdoutCount != 2 {
		t.Fatalf("stdout_count=%d, want 2", stdoutCount)
	}
	if assistantCount != 1 {
		t.Fatalf("assistant_count=%d, want 1", assistantCount)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.SessionID != "sess-1" {
		t.Fatalf("session_id=%q, want %q", updated.SessionID, "sess-1")
	}
	if strings.TrimSpace(sid) != "sess-1" {
		t.Fatalf("sid=%q, want %q", sid, "sess-1")
	}
}

func TestManager_consumeStdout_ClaudeCode_StoresRawWhenParseFails(t *testing.T) {
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
		Prompt:     "x",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := &Manager{cfg: config.Default(), store: store}

	var (
		sidMu sync.Mutex
		sid   string
	)
	m.consumeStdout(task, tasks.WorkerClaudeCode, strings.NewReader("not-json\n"), &sidMu, &sid, func() {}, &resumeFailureState{}, &blockedState{})

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}

	stdoutCount := 0
	for _, l := range logs {
		if l.Stream == tasks.LogStdout {
			stdoutCount++
		}
	}
	if stdoutCount != 1 {
		t.Fatalf("stdout_count=%d, want 1", stdoutCount)
	}
}

func TestManager_consumeStdout_Codex_StoresRawStdout(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := &Manager{cfg: config.Default(), store: store}

	var (
		sidMu sync.Mutex
		sid   string
	)
	out := strings.Join([]string{
		`{"type":"noop","thread_id":"thr-1"}`,
		`{"type":"item.completed","thread_id":"thr-1","item":{"type":"agent_message","text":"hello"}}`,
	}, "\n")

	m.consumeStdout(task, tasks.WorkerCodex, strings.NewReader(out), &sidMu, &sid, func() {}, &resumeFailureState{}, &blockedState{})

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}

	stdoutCount := 0
	assistantCount := 0
	for _, l := range logs {
		if l.Stream == tasks.LogStdout {
			stdoutCount++
		}
		if l.Stream == tasks.LogAssistant {
			assistantCount++
		}
	}
	if stdoutCount != 2 {
		t.Fatalf("stdout_count=%d, want 2", stdoutCount)
	}
	if assistantCount != 1 {
		t.Fatalf("assistant_count=%d, want 1", assistantCount)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.SessionID != "thr-1" {
		t.Fatalf("session_id=%q, want %q", updated.SessionID, "thr-1")
	}
	if strings.TrimSpace(sid) != "thr-1" {
		t.Fatalf("sid=%q, want %q", sid, "thr-1")
	}
}
