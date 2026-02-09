package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestCCXCodexHelperProcess(t *testing.T) {
	// Helper process entrypoint: when the worker launches the test binary as "codex",
	// tools.go prefixes args with `-test.run=TestCCXCodexHelperProcess -- ccx-helper-codex`.
	args := flag.Args()
	if len(args) == 0 || args[0] != "ccx-helper-codex" {
		return
	}
	runFakeCodexAppServer()
	os.Exit(0)
}

func runFakeCodexAppServer() {
	stdout := bufio.NewWriter(os.Stdout)
	defer func() { _ = stdout.Flush() }()

	writeJSONLine := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = stdout.Write(b)
		_, _ = stdout.WriteString("\n")
		_ = stdout.Flush()
	}

	readNext := func(sc *bufio.Scanner) (map[string]any, bool) {
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var msg map[string]any
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			return msg, true
		}
		return nil, false
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	// 1) initialize
	for {
		msg, ok := readNext(sc)
		if !ok {
			return
		}
		if msg["method"] != "initialize" {
			continue
		}
		id, ok := msg["id"]
		if !ok {
			continue
		}
		writeJSONLine(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  map[string]any{"userAgent": "fake"},
		})
		break
	}

	// 2) initialized notification
	for {
		msg, ok := readNext(sc)
		if !ok {
			return
		}
		if msg["method"] == "initialized" {
			break
		}
	}

	const threadID = "thr-1"
	const turnID = "turn-1"

	// 3) thread/start or thread/resume
	for {
		msg, ok := readNext(sc)
		if !ok {
			return
		}
		method, _ := msg["method"].(string)
		if method != "thread/start" && method != "thread/resume" {
			continue
		}
		id, ok := msg["id"]
		if !ok {
			continue
		}
		writeJSONLine(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  map[string]any{"thread": map[string]any{"id": threadID}},
		})
		break
	}

	// 4) turn/start -> respond -> request approval -> wait -> complete
	for {
		msg, ok := readNext(sc)
		if !ok {
			return
		}
		if msg["method"] != "turn/start" {
			continue
		}
		id, ok := msg["id"]
		if !ok {
			continue
		}
		writeJSONLine(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  map[string]any{"turn": map[string]any{"id": turnID}},
		})
		break
	}

	const approvalReqID = "srv-1"
	writeJSONLine(map[string]any{
		"jsonrpc": "2.0",
		"id":      approvalReqID,
		"method":  "item/commandExecution/requestApproval",
		"params": map[string]any{
			"itemId":   "item-1",
			"threadId": threadID,
			"turnId":   turnID,
			"command":  "echo hello",
			"cwd":      ".",
		},
	})

	decision := "decline"
	for {
		msg, ok := readNext(sc)
		if !ok {
			return
		}
		if msg["id"] != approvalReqID {
			continue
		}
		res, _ := msg["result"].(map[string]any)
		if d, ok := res["decision"].(string); ok {
			decision = strings.TrimSpace(d)
		}
		break
	}

	reply := "denied"
	if decision == "accept" || decision == "acceptForSession" {
		reply = "ok"
	}

	writeJSONLine(map[string]any{
		"jsonrpc": "2.0",
		"method":  "item/completed",
		"params": map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
			"item": map[string]any{
				"id":   "it-1",
				"type": "agentMessage",
				"text": reply,
			},
		},
	})
	writeJSONLine(map[string]any{
		"jsonrpc": "2.0",
		"method":  "turn/completed",
		"params":  map[string]any{"threadId": threadID, "turnId": turnID},
	})
}

func TestManager_run_Codex_AppServerApproval_ApproveContinues(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CODEX_HELPER_PROCESS", "1")

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
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Codex = os.Args[0]

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
		t.Fatalf("status=%q, want %q (error=%q)", updated.Status, tasks.StatusSucceeded, updated.Error)
	}
	if strings.TrimSpace(updated.SessionID) != "thr-1" {
		t.Fatalf("session_id=%q, want %q", updated.SessionID, "thr-1")
	}
}

func TestManager_run_Codex_AppServerApproval_CancelExpiresApproval(t *testing.T) {
	t.Setenv("CONTROLCCX_TEST_CODEX_HELPER_PROCESS", "1")

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
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "x",
		WorkDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Codex = os.Args[0]

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
		t.Fatalf("status=%q, want %q (error=%q)", updated.Status, tasks.StatusCanceled, updated.Error)
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
