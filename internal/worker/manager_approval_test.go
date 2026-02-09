package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestCCXClaudeHelperProcess(t *testing.T) {
	// Helper process entrypoint: when the worker launches the test binary as "claude",
	// tools.go prefixes args with `-test.run=TestCCXClaudeHelperProcess -- ccx-helper-claude`.
	args := flag.Args()
	if len(args) == 0 || args[0] != "ccx-helper-claude" {
		return
	}

	runFakeClaudeProtocol()
	os.Exit(0)
}

func runFakeClaudeProtocol() {
	stdout := bufio.NewWriter(os.Stdout)
	defer func() { _ = stdout.Flush() }()
	stderr := bufio.NewWriter(os.Stderr)
	defer func() { _ = stderr.Flush() }()

	writeLine := func(s string) {
		_, _ = stdout.WriteString(s)
		_, _ = stdout.WriteString("\n")
		_ = stdout.Flush()
	}

	const (
		sessionID = "sess-1"
		reqID     = "req-1"
	)
	waitForEOF := strings.TrimSpace(os.Getenv("CONTROLCCX_TEST_CLAUDE_HELPER_WAIT_EOF")) != ""

	// Emit init early so the worker can persist session_id.
	writeLine(`{"type":"system","subtype":"init","session_id":"` + sessionID + `"}`)

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	seenUser := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		typ, _ := msg["type"].(string)
		if typ == "user" {
			seenUser = true
			break
		}
	}
	if err := sc.Err(); err != nil {
		_, _ = fmt.Fprintf(stderr, "stdin scan error: %v\n", err)
		return
	}
	if !seenUser {
		return
	}

	// Request tool approval and block until we receive a control_response.
	writeLine(`{"type":"control_request","request_id":"` + reqID + `","request":{"subtype":"can_use_tool","tool_name":"WebSearch","input":{"q":"weather","description":"Get weather"},"tool_use_id":"toolu-1"}}`)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var msg struct {
			Type     string `json:"type"`
			Response struct {
				Subtype   string `json:"subtype"`
				RequestID string `json:"request_id"`
				Response  struct {
					Behavior string `json:"behavior"`
					Message  string `json:"message,omitempty"`
				} `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Type != "control_response" {
			continue
		}
		if msg.Response.Subtype != "success" || msg.Response.RequestID != reqID {
			continue
		}
		behavior := strings.ToLower(strings.TrimSpace(msg.Response.Response.Behavior))
		switch behavior {
		case "allow":
			writeLine(`{"type":"result","subtype":"success","session_id":"` + sessionID + `","result":"ok","is_error":false}`)
			if waitForEOF {
				for sc.Scan() {
				}
			}
			return
		default:
			// Deny or unknown: return success so the worker can finish.
			writeLine(`{"type":"result","subtype":"success","session_id":"` + sessionID + `","result":"denied","is_error":false}`)
			if waitForEOF {
				for sc.Scan() {
				}
			}
			return
		}
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

func TestManager_run_ClaudeCode_ProtocolApproval_ApproveContinues(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_PROCESS", "1")
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_WAIT_EOF", "1")

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

	cfg := config.Default()
	cfg.Paths.Claude = os.Args[0]

	m := NewManager(cfg, store, nil, nil, nil)
	m.approvalTimeout = 2 * time.Second

	done := make(chan error, 1)
	go func() {
		done <- m.run(ctx, task)
	}()

	awaitTaskStatus(t, store, task.ID, tasks.StatusAwaitingApproval, 2*time.Second)
	approval := awaitPendingApproval(t, store, task.ID, 2*time.Second)

	if err := store.UpdateApprovalRequestDecision(ctx, approval.ID, tasks.UpdateApprovalRequestDecisionInput{
		Status: tasks.ApprovalStatusApproved,
		Reason: "ok",
	}); err != nil {
		t.Fatalf("UpdateApprovalRequestDecision: %v", err)
	}
	if ok := m.SubmitApprovalDecision(task.ID, approval.ID, "approve", "ok"); !ok {
		t.Fatalf("SubmitApprovalDecision ok=false, want true")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for run to finish")
	}

	updated := awaitTaskTerminalStatus(t, store, task.ID, 2*time.Second)
	if updated.Status != tasks.StatusSucceeded {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusSucceeded, updated.Warning, updated.Error)
	}

	ar, ok, err := store.GetApprovalRequest(ctx, approval.ID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if !ok {
		t.Fatalf("expected approval request")
	}
	if ar.Status != tasks.ApprovalStatusApproved || strings.TrimSpace(ar.Reason) != "ok" {
		t.Fatalf("approval status=%q reason=%q, want %q/%q", ar.Status, ar.Reason, tasks.ApprovalStatusApproved, "ok")
	}
}

func TestManager_run_ClaudeCode_ProtocolApproval_TimeoutExpires(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_PROCESS", "1")
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_WAIT_EOF", "1")

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

	cfg := config.Default()
	cfg.Paths.Claude = os.Args[0]

	m := NewManager(cfg, store, nil, nil, nil)
	m.approvalTimeout = 50 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- m.run(ctx, task)
	}()

	awaitTaskStatus(t, store, task.ID, tasks.StatusAwaitingApproval, 2*time.Second)
	approval := awaitPendingApproval(t, store, task.ID, 2*time.Second)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for run to finish")
	}

	updated := awaitTaskTerminalStatus(t, store, task.ID, 2*time.Second)
	if updated.Status != tasks.StatusSucceeded {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusSucceeded, updated.Warning, updated.Error)
	}

	ar, ok, err := store.GetApprovalRequest(ctx, approval.ID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if !ok {
		t.Fatalf("expected approval request")
	}
	if ar.Status != tasks.ApprovalStatusExpired {
		t.Fatalf("approval status=%q, want %q (reason=%q)", ar.Status, tasks.ApprovalStatusExpired, ar.Reason)
	}
	if !strings.Contains(strings.ToLower(ar.Reason), "timed out") {
		t.Fatalf("approval reason=%q, want contains %q", ar.Reason, "timed out")
	}

	// A late decision should not panic/hang.
	_ = store.UpdateApprovalRequestDecision(ctx, approval.ID, tasks.UpdateApprovalRequestDecisionInput{
		Status: tasks.ApprovalStatusDenied,
		Reason: "late",
	})
	_ = m.SubmitApprovalDecision(task.ID, approval.ID, "deny", "late")
}

func TestManager_run_ClaudeCode_ProtocolApproval_CancelExpiresApproval(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_PROCESS", "1")
	t.Setenv("CONTROLCCX_TEST_CLAUDE_HELPER_WAIT_EOF", "1")

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "controlccx.db")
	conn, err := db.Open(runCtx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	task, err := store.CreateTask(runCtx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Claude = os.Args[0]

	m := NewManager(cfg, store, nil, nil, nil)
	m.approvalTimeout = 5 * time.Minute

	done := make(chan error, 1)
	go func() {
		done <- m.run(runCtx, task)
	}()

	awaitTaskStatus(t, store, task.ID, tasks.StatusAwaitingApproval, 2*time.Second)
	approval := awaitPendingApproval(t, store, task.ID, 2*time.Second)

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for run to finish")
	}

	updated := awaitTaskTerminalStatus(t, store, task.ID, 2*time.Second)
	if updated.Status != tasks.StatusCanceled {
		t.Fatalf("status=%q, want %q (warning=%q error=%q)", updated.Status, tasks.StatusCanceled, updated.Warning, updated.Error)
	}

	ar, ok, err := store.GetApprovalRequest(context.Background(), approval.ID)
	if err != nil {
		t.Fatalf("GetApprovalRequest: %v", err)
	}
	if !ok {
		t.Fatalf("expected approval request")
	}
	if ar.Status != tasks.ApprovalStatusExpired {
		t.Fatalf("approval status=%q, want %q (reason=%q)", ar.Status, tasks.ApprovalStatusExpired, ar.Reason)
	}
	if !strings.Contains(strings.ToLower(ar.Reason), "cancel") {
		t.Fatalf("approval reason=%q, want contains %q", ar.Reason, "cancel")
	}
}

func awaitPendingApproval(t *testing.T, store *tasks.Store, taskID string, timeout time.Duration) tasks.ApprovalRequest {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		list, err := store.ListApprovalRequestsByTask(context.Background(), taskID, tasks.ListApprovalRequestsOptions{
			Status: tasks.ApprovalStatusPending,
		})
		if err != nil {
			t.Fatalf("ListApprovalRequestsByTask: %v", err)
		}
		if len(list) > 0 {
			return list[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	list, _ := store.ListApprovalRequestsByTask(context.Background(), taskID, tasks.ListApprovalRequestsOptions{})
	t.Fatalf("timeout waiting for pending approval (got %d)", len(list))
	return tasks.ApprovalRequest{}
}

func awaitTaskStatus(t *testing.T, store *tasks.Store, taskID string, want tasks.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := store.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		if task.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := store.GetTask(context.Background(), taskID)
	t.Fatalf("timeout waiting for task %s status=%s (got=%s)", taskID, want, task.Status)
}

func awaitTaskTerminalStatus(t *testing.T, store *tasks.Store, taskID string, timeout time.Duration) tasks.Task {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := store.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", taskID, err)
		}
		switch task.Status {
		case tasks.StatusQueued, tasks.StatusWaiting, tasks.StatusRunning, tasks.StatusAwaitingApproval:
			time.Sleep(10 * time.Millisecond)
			continue
		default:
			return task
		}
	}
	task, _ := store.GetTask(context.Background(), taskID)
	t.Fatalf("timeout waiting for task %s terminal status (status=%s)", taskID, task.Status)
	return tasks.Task{}
}

func TestApprovalRequests_CRUD(t *testing.T) {
	// Ensure approvals store methods behave as expected, especially for "not pending" errors.
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
	ar, err := store.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: task.WorkerType,
		WorkDir:    task.WorkDir,
		ActionType: "shell.exec",
		RiskLevel:  tasks.RiskHigh,
		Summary:    "run command",
		Raw:        []byte(`{"tool":"bash","command":"echo hi"}`),
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	if err := store.UpdateApprovalRequestDecision(ctx, ar.ID, tasks.UpdateApprovalRequestDecisionInput{
		Status: tasks.ApprovalStatusApproved,
		Reason: "ok",
	}); err != nil {
		t.Fatalf("UpdateApprovalRequestDecision: %v", err)
	}
	if err := store.UpdateApprovalRequestDecision(ctx, ar.ID, tasks.UpdateApprovalRequestDecisionInput{
		Status: tasks.ApprovalStatusDenied,
		Reason: "nope",
	}); err == nil {
		t.Fatalf("expected second decision to fail")
	} else {
		var notPending *tasks.ApprovalNotPendingError
		if !errors.As(err, &notPending) {
			t.Fatalf("err=%T want *ApprovalNotPendingError", err)
		}
	}
}
