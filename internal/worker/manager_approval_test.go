package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestManager_run_ClaudeCode_RequiresApproval_BecomesBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: This command requires approval","is_error":true}]},"session_id":"sess-1"}'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}
	if !strings.Contains(strings.ToLower(updated.Warning), "approval") {
		t.Fatalf("warning=%q, want contains %q", updated.Warning, "approval")
	}
	if strings.TrimSpace(updated.Error) != "" {
		t.Fatalf("error=%q, want empty", updated.Error)
	}

	logs, err := store.ListLogs(ctx, task.ID, 0, 2000)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	found := false
	for _, l := range logs {
		if l.Stream != tasks.LogSystem {
			continue
		}
		if strings.Contains(strings.ToLower(l.Message), "requires approval") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected system log mentioning requires approval")
	}
}

func TestIsApprovalRequiredLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "requires_approval",
			line: `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: This command requires approval","is_error":true}]},"session_id":"sess-1"}`,
			want: true,
		},
		{
			name: "requested_permissions",
			line: `Claude requested permissions to write to index.html, but you haven't granted it yet.`,
			want: true,
		},
		{
			name: "normal_text",
			line: "boom",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isApprovalRequiredLine([]byte(tt.line)); got != tt.want {
				t.Fatalf("got=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestManager_run_ClaudeCode_NormalFailure_RemainsFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo '{"type":"assistant","session_id":"sess-1","result":"boom"}'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusFailed {
		t.Fatalf("status=%q, want %q", updated.Status, tasks.StatusFailed)
	}
}

func TestManager_run_ClaudeCode_RequestedPermissions_BecomesBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo 'Claude requested permissions to write to index.html, but you haven'"'"'t granted it yet.'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := &Manager{cfg: cfg, store: store}
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		logs, _ := store.ListLogs(ctx, task.ID, 0, 2000)
		var tail []string
		for _, l := range logs {
			tail = append(tail, string(l.Stream)+": "+l.Message)
		}
		if len(tail) > 20 {
			tail = tail[len(tail)-20:]
		}
		t.Fatalf("status=%q, want %q (warning=%q error=%q logs_tail=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error, tail)
	}
}

func TestManager_run_ClaudeCode_RequiresApproval_AutoContinuesWhenSafe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`case "$*" in`,
		`  *--dangerously-skip-permissions*)`,
		`    echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`    echo '{"type":"assistant","session_id":"sess-1","result":"ok"}'`,
		`    exit 0`,
		`    ;;`,
		`esac`,
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"Error: This command requires approval","is_error":true}]},"session_id":"sess-1"}'`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := NewManager(cfg, store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}

	runs := awaitConversationTaskCount(t, store, strings.TrimSpace(task.ConversationID), 2, 2*time.Second)
	latest := runs[0]
	if latest.ID == task.ID {
		t.Fatalf("expected a follow-up task; got latest=%s", latest.ID)
	}
	if latest.UnsafeAutomation != true {
		t.Fatalf("latest unsafe_automation=%v, want true", latest.UnsafeAutomation)
	}
	if latest.Mode != tasks.ModeResume {
		t.Fatalf("latest mode=%q, want %q", latest.Mode, tasks.ModeResume)
	}

	latest = awaitTaskTerminal(t, store, latest.ID, 2*time.Second)
	if latest.Status != tasks.StatusSucceeded {
		t.Fatalf("latest status=%q, want %q (warning=%q error=%q)", latest.Status, tasks.StatusSucceeded, latest.Warning, latest.Error)
	}
}

func TestManager_run_ClaudeCode_RequiresApproval_DeleteDir_RemainsBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo "about to run: rm -rf some_dir" 1>&2`,
		`echo "Error: This command requires approval" 1>&2`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := NewManager(cfg, store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}

	assertConversationTaskCountNotExceed(t, store, strings.TrimSpace(task.ConversationID), 1, 600*time.Millisecond)
}

func TestManager_run_ClaudeCode_RequiresApproval_SystemRisk_RemainsBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claude-code tests use unix shell scripts")
	}

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	claude := filepath.Join(t.TempDir(), "fake-claude")
	script := strings.Join([]string{
		"#!/bin/sh",
		`echo '{"type":"system","subtype":"init","session_id":"sess-1"}'`,
		`echo "sudo reboot" 1>&2`,
		`echo "Error: This command requires approval" 1>&2`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(claude, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = claude

	m := NewManager(cfg, store, nil, nil, nil)
	if err := m.run(ctx, task); err != nil {
		t.Fatalf("run: %v", err)
	}

	updated, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != tasks.StatusBlocked {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusBlocked, updated.Warning, updated.Error)
	}

	assertConversationTaskCountNotExceed(t, store, strings.TrimSpace(task.ConversationID), 1, 600*time.Millisecond)
}

func awaitConversationTaskCount(t *testing.T, store *tasks.Store, conversationID string, want int, timeout time.Duration) []tasks.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs, err := store.ListTasksByConversationID(context.Background(), conversationID, 50, tasks.ListTasksOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("ListTasksByConversationID: %v", err)
		}
		if len(runs) >= want {
			return runs
		}
		time.Sleep(20 * time.Millisecond)
	}
	runs, _ := store.ListTasksByConversationID(context.Background(), conversationID, 50, tasks.ListTasksOptions{IncludeDeleted: true})
	t.Fatalf("timeout waiting for %d tasks in conversation %q (got %d)", want, conversationID, len(runs))
	return nil
}

func awaitTaskTerminal(t *testing.T, store *tasks.Store, taskID string, timeout time.Duration) tasks.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := store.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		switch task.Status {
		case tasks.StatusQueued, tasks.StatusRunning:
			time.Sleep(20 * time.Millisecond)
			continue
		default:
			return task
		}
	}
	task, _ := store.GetTask(context.Background(), taskID)
	t.Fatalf("timeout waiting for task %s terminal status (status=%s)", taskID, task.Status)
	return tasks.Task{}
}

func assertConversationTaskCountNotExceed(t *testing.T, store *tasks.Store, conversationID string, max int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs, err := store.ListTasksByConversationID(context.Background(), conversationID, 50, tasks.ListTasksOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("ListTasksByConversationID: %v", err)
		}
		if len(runs) > max {
			t.Fatalf("expected at most %d tasks in conversation %q, got %d", max, conversationID, len(runs))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
