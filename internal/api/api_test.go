package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

func TestAPI_TasksAndChat(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:    taskStore,
		Workers:  nil,
		Observer: &observer.Service{Store: taskStore},
		Chat:     chatStore,
		Hub:      hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	t.Run("create task", func(t *testing.T) {
		body := tasks.CreateTaskInput{
			WorkerType: tasks.WorkerExec,
			Mode:       tasks.ModeNew,
			Prompt:     "echo hello",
			WorkDir:    ".",
		}
		buf, _ := json.Marshal(body)
		res, err := http.Post(srv.URL+"/api/tasks", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}

		var created tasks.Task
		if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if created.ID == "" {
			t.Fatalf("expected id")
		}

		_, _ = taskStore.AppendLog(ctx, created.ID, tasks.LogStdout, "hello")
		logRes, err := http.Get(srv.URL + "/api/tasks/" + created.ID + "/logs?after=0&limit=10")
		if err != nil {
			t.Fatalf("get logs: %v", err)
		}
		defer logRes.Body.Close()
		if logRes.StatusCode != http.StatusOK {
			t.Fatalf("logs status=%d, want 200", logRes.StatusCode)
		}
		var logsBody struct {
			Logs []tasks.LogEntry `json:"logs"`
		}
		if err := json.NewDecoder(logRes.Body).Decode(&logsBody); err != nil {
			t.Fatalf("decode logs: %v", err)
		}
		if len(logsBody.Logs) != 1 || logsBody.Logs[0].Message != "hello" {
			t.Fatalf("unexpected logs: %+v", logsBody.Logs)
		}

		_ = taskStore.SetSessionID(ctx, created.ID, "sess-1")
		resumeReq := map[string]string{"prompt": "continue"}
		resumeBuf, _ := json.Marshal(resumeReq)
		resumeRes, err := http.Post(srv.URL+"/api/tasks/"+created.ID+"/resume", "application/json", bytes.NewReader(resumeBuf))
		if err != nil {
			t.Fatalf("resume post: %v", err)
		}
		defer resumeRes.Body.Close()
		if resumeRes.StatusCode != http.StatusOK {
			t.Fatalf("resume status=%d, want 200", resumeRes.StatusCode)
		}
		var resumed tasks.Task
		if err := json.NewDecoder(resumeRes.Body).Decode(&resumed); err != nil {
			t.Fatalf("decode resumed: %v", err)
		}
		if resumed.Mode != tasks.ModeResume || resumed.SessionID != "sess-1" {
			t.Fatalf("resumed mode=%q session=%q", resumed.Mode, resumed.SessionID)
		}

		getRes, err := http.Get(srv.URL + "/api/tasks/" + created.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer getRes.Body.Close()
		if getRes.StatusCode != http.StatusOK {
			t.Fatalf("get status=%d, want 200", getRes.StatusCode)
		}
	})

	t.Run("chat", func(t *testing.T) {
		req := map[string]string{"message": "我们有几个任务在执行？"}
		buf, _ := json.Marshal(req)
		res, err := http.Post(srv.URL+"/api/chat", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var reply map[string]string
		if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if reply["message"] == "" {
			t.Fatalf("expected reply message")
		}
	})

	t.Run("fs", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "root")
		if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		apiSvc.FSRoots = []FSRoot{{Name: "Root", Path: root}}

		res, err := http.Get(srv.URL + "/api/fs/roots")
		if err != nil {
			t.Fatalf("get roots: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}

		listRes, err := http.Get(srv.URL + "/api/fs/list?path=" + root)
		if err != nil {
			t.Fatalf("get list: %v", err)
		}
		defer listRes.Body.Close()
		if listRes.StatusCode != http.StatusOK {
			t.Fatalf("list status=%d, want 200", listRes.StatusCode)
		}
		var listed FSListResponse
		if err := json.NewDecoder(listRes.Body).Decode(&listed); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(listed.Entries) != 2 {
			t.Fatalf("entries=%d, want 2", len(listed.Entries))
		}

		outside := filepath.Dir(root)
		outRes, err := http.Get(srv.URL + "/api/fs/list?path=" + outside)
		if err != nil {
			t.Fatalf("get outside: %v", err)
		}
		defer outRes.Body.Close()
		if outRes.StatusCode != http.StatusForbidden {
			t.Fatalf("outside status=%d, want 403", outRes.StatusCode)
		}
	})
}
