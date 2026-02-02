package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
)

type stubObserverBackend struct {
	i       int
	outputs []string
}

func (s *stubObserverBackend) Name() string { return "stub" }

func (s *stubObserverBackend) Complete(ctx context.Context, prompt string) (string, error) {
	if s == nil {
		return "", nil
	}
	if s.i >= len(s.outputs) {
		return `{"action":"final","message":"done"}`, nil
	}
	out := s.outputs[s.i]
	s.i++
	return out, nil
}

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
		Observer: &observer.Service{Store: taskStore, Chat: chatStore},
		Chat:     chatStore,
		Hub:      hub,
	}

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	t.Run("create task", func(t *testing.T) {
		baseDir := t.TempDir()
		body := tasks.CreateTaskInput{
			WorkerType: tasks.WorkerExec,
			Mode:       tasks.ModeNew,
			Prompt:     "echo hello",
			WorkDir:    baseDir,
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
		exitCode := 0
		if err := taskStore.FinishTask(ctx, created.ID, tasks.FinishTaskInput{
			Status:     tasks.StatusSucceeded,
			ExitCode:   &exitCode,
			Error:      "",
			SessionID:  "sess-1",
			FinishedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("finish created: %v", err)
		}
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

	t.Run("acceptance", func(t *testing.T) {
		baseDir := t.TempDir()
		// Create a task so we can reference via session key.
		task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
			WorkerType: tasks.WorkerExec,
			Mode:       tasks.ModeNew,
			Prompt:     "hi",
			WorkDir:    baseDir,
			SessionID:  "sess-acc-1",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		_, err = taskStore.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
			Key:           "s:sess-acc-1",
			Status:        "running",
			Iteration:     2,
			MaxIterations: 10,
			CurrentGate:   "runnability.smoke",
			Summary:       "in progress",
			RunID:         task.ID,
		})
		if err != nil {
			t.Fatalf("upsert acceptance: %v", err)
		}

		res, err := http.Get(srv.URL + "/api/acceptance?key=" + url.QueryEscape("s:sess-acc-1"))
		if err != nil {
			t.Fatalf("get acceptance: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("status=%d, want 200; body=%s", res.StatusCode, string(body))
		}
		var body struct {
			OK    bool                   `json:"ok"`
			State *tasks.AcceptanceState `json:"state"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.OK || body.State == nil || body.State.Key != "s:sess-acc-1" {
			t.Fatalf("unexpected response: %+v", body)
		}

		// task_id variant
		res2, err := http.Get(srv.URL + "/api/acceptance?task_id=" + url.QueryEscape(task.ID))
		if err != nil {
			t.Fatalf("get acceptance by task: %v", err)
		}
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res2.Body)
			t.Fatalf("status=%d, want 200; body=%s", res2.StatusCode, string(body))
		}
	})

	t.Run("chat stream", func(t *testing.T) {
		sb := &stubObserverBackend{
			outputs: []string{
				`{"action":"tool","tool":"system_info","args":{}}`,
				`{"action":"final","message":"ok"}`,
			},
		}
		var _ observer.Backend = sb

		// Replace observer service with an LLM-backed agent for this test.
		apiSvc.Observer = &observer.Service{
			Store:      taskStore,
			Chat:       chatStore,
			LLM:        sb,
			ForceAgent: true,
		}

		req := map[string]any{"message": "hello", "stream": true}
		buf, _ := json.Marshal(req)
		httpReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/chat?stream=1", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("new req: %v", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		res, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("status=%d, want 200; body=%s", res.StatusCode, string(body))
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			t.Fatalf("content-type=%q, want text/event-stream", ct)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		s := string(body)
		if !strings.Contains(s, "event: tool_call") {
			t.Fatalf("expected tool_call event, got:\n%s", s)
		}
		if !strings.Contains(s, "event: tool_result") {
			t.Fatalf("expected tool_result event, got:\n%s", s)
		}
		if !strings.Contains(s, "event: final") {
			t.Fatalf("expected final event, got:\n%s", s)
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
		if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
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

		readURL := srv.URL + "/api/fs/read?path=" + url.QueryEscape("note.md") + "&base=" + url.QueryEscape(root)
		readRes, err := http.Get(readURL)
		if err != nil {
			t.Fatalf("get read: %v", err)
		}
		defer readRes.Body.Close()
		if readRes.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(readRes.Body)
			t.Fatalf("read status=%d, want 200; body=%s", readRes.StatusCode, string(body))
		}
		var readBody struct {
			Path      string `json:"path"`
			Size      int64  `json:"size"`
			Truncated bool   `json:"truncated"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(readRes.Body).Decode(&readBody); err != nil {
			t.Fatalf("decode read: %v", err)
		}
		if !strings.HasSuffix(readBody.Path, string(filepath.Separator)+"note.md") {
			t.Fatalf("read path=%q, want suffix note.md", readBody.Path)
		}
		if readBody.Size <= 0 {
			t.Fatalf("read size=%d, want >0", readBody.Size)
		}
		if readBody.Truncated {
			t.Fatalf("read truncated=true, want false")
		}
		if readBody.Content != "# Hello\n" {
			t.Fatalf("read content=%q, want %q", readBody.Content, "# Hello\n")
		}

		readDirRes, err := http.Get(srv.URL + "/api/fs/read?path=" + url.QueryEscape(filepath.Join(root, "a")))
		if err != nil {
			t.Fatalf("get read dir: %v", err)
		}
		defer readDirRes.Body.Close()
		if readDirRes.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(readDirRes.Body)
			t.Fatalf("read dir status=%d, want 400; body=%s", readDirRes.StatusCode, string(body))
		}
	})

	t.Run("auth", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("CODEX_HOME", t.TempDir())
		t.Setenv("HOME", t.TempDir())

		store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
		if err != nil {
			t.Fatalf("auth load: %v", err)
		}
		apiSvc.Auth = store

		res, err := http.Get(srv.URL + "/api/auth/status")
		if err != nil {
			t.Fatalf("get status: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var st auth.Status
		if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if st.Claude.Available || st.Codex.Available {
			t.Fatalf("expected unavailable, got claude=%v codex=%v", st.Claude.Available, st.Codex.Available)
		}

		openaiKey := "sk-openai-abc123"
		buf, _ := json.Marshal(map[string]string{"openai_api_key": openaiKey})
		setRes, err := http.Post(srv.URL+"/api/auth", "application/json", bytes.NewReader(buf))
		if err != nil {
			t.Fatalf("post auth: %v", err)
		}
		defer setRes.Body.Close()
		if setRes.StatusCode != http.StatusOK {
			t.Fatalf("set status=%d, want 200", setRes.StatusCode)
		}
		var setBody struct {
			Status auth.Status `json:"status"`
		}
		if err := json.NewDecoder(setRes.Body).Decode(&setBody); err != nil {
			t.Fatalf("decode set: %v", err)
		}
		if !setBody.Status.Codex.Available || setBody.Status.Codex.APIKey.Effective != "stored" {
			t.Fatalf("unexpected set status: %+v", setBody.Status.Codex)
		}
	})
}
