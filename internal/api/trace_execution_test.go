package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_TraceAndLogQueryExport(t *testing.T) {
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
		Prompt:     "echo hi",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, _ = store.AppendLog(ctx, task.ID, tasks.LogStdout, "alpha one")
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogStderr, "beta two")
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, "alpha three")
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogSystem, "gamma four")

	if err := store.SetInvocation(ctx, task.ID, "claude", []string{"-p", "-"}, "/x", []string{"ANTHROPIC_API_KEY"}); err != nil {
		t.Fatalf("SetInvocation: %v", err)
	}

	apiSvc := &API{Tasks: store}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/tasks/" + task.ID + "/trace")
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("trace status=%d, want 200", res.StatusCode)
	}
	var trace struct {
		Task       tasks.Task        `json:"task"`
		Invocation *tasks.Invocation `json:"invocation"`
	}
	if err := json.NewDecoder(res.Body).Decode(&trace); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if trace.Invocation == nil || trace.Invocation.Cmd != "claude" {
		t.Fatalf("invocation=%#v", trace.Invocation)
	}
	if len(trace.Invocation.EnvInjectedKeys) != 1 || trace.Invocation.EnvInjectedKeys[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("env_keys=%v", trace.Invocation.EnvInjectedKeys)
	}

	logRes, err := http.Get(srv.URL + "/api/tasks/" + task.ID + "/logs?streams=stdout,assistant&q=alpha&limit=50")
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}
	defer logRes.Body.Close()
	if logRes.StatusCode != http.StatusOK {
		t.Fatalf("logs status=%d, want 200", logRes.StatusCode)
	}
	var logsOut struct {
		Logs []tasks.LogEntry `json:"logs"`
	}
	if err := json.NewDecoder(logRes.Body).Decode(&logsOut); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if len(logsOut.Logs) != 2 {
		t.Fatalf("logs=%v", logsOut.Logs)
	}
	for _, l := range logsOut.Logs {
		if !strings.Contains(l.Message, "alpha") {
			t.Fatalf("unexpected message: %q", l.Message)
		}
		if l.Stream != tasks.LogStdout && l.Stream != tasks.LogAssistant {
			t.Fatalf("unexpected stream: %s", l.Stream)
		}
	}

	expRes, err := http.Get(srv.URL + "/api/tasks/" + task.ID + "/logs/export?streams=stdout,assistant&q=alpha")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	defer expRes.Body.Close()
	if expRes.StatusCode != http.StatusOK {
		t.Fatalf("export status=%d, want 200", expRes.StatusCode)
	}
	body, _ := io.ReadAll(expRes.Body)
	text := string(body)
	if !strings.Contains(text, "alpha one") || !strings.Contains(text, "alpha three") {
		t.Fatalf("export missing expected lines: %q", text)
	}
	if strings.Contains(text, "beta two") || strings.Contains(text, "gamma four") {
		t.Fatalf("export should exclude filtered lines: %q", text)
	}
}

