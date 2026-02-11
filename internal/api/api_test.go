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
	"controlccx/internal/db"
	"controlccx/internal/events"
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
	hub := events.NewHub()

	apiSvc := &API{
		Tasks:   taskStore,
		Workers: nil,
		Hub:     hub,
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

		createOut := decodeMutationResponse(t, res)
		requireMutationAction(t, createOut, "task.create")
		created := requireMutationTask(t, createOut)
		if created.ID == "" {
			t.Fatalf("expected id")
		}
		if created.NetworkTier != tasks.NetworkTierWebReadonly {
			t.Fatalf("created network_tier=%q, want %q", created.NetworkTier, tasks.NetworkTierWebReadonly)
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
		resumeOut := decodeMutationResponse(t, resumeRes)
		requireMutationAction(t, resumeOut, "task.resume")
		resumed := requireMutationTask(t, resumeOut)
		if resumed.Mode != tasks.ModeResume || resumed.SessionID != "sess-1" {
			t.Fatalf("resumed mode=%q session=%q", resumed.Mode, resumed.SessionID)
		}
		if resumed.NetworkTier != created.NetworkTier {
			t.Fatalf("resumed network_tier=%q, want %q", resumed.NetworkTier, created.NetworkTier)
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

	t.Run("mission contract", func(t *testing.T) {
		baseDir := t.TempDir()
		task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
			WorkerType: tasks.WorkerExec,
			Mode:       tasks.ModeNew,
			Prompt:     "draft mission contract",
			WorkDir:    baseDir,
			SessionID:  "sess-contract-1",
		})
		if err != nil {
			t.Fatalf("create task: %v", err)
		}

		missingRes, err := http.Get(srv.URL + "/api/mission-contract?key=" + url.QueryEscape("c:missing-contract"))
		if err != nil {
			t.Fatalf("get missing mission contract: %v", err)
		}
		defer missingRes.Body.Close()
		if missingRes.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(missingRes.Body)
			t.Fatalf("missing status=%d, want 200; body=%s", missingRes.StatusCode, string(body))
		}
		var missingBody struct {
			OK       bool                   `json:"ok"`
			Contract *tasks.MissionContract `json:"contract"`
		}
		if err := json.NewDecoder(missingRes.Body).Decode(&missingBody); err != nil {
			t.Fatalf("decode missing body: %v", err)
		}
		if missingBody.OK || missingBody.Contract != nil {
			t.Fatalf("unexpected missing response: %+v", missingBody)
		}

		createPayload := map[string]any{
			"task_id": task.ID,
			"goal":    "Deliver autonomous execution loop safely",
			"constraints": []string{
				"always run tests before commit",
				"no destructive commands",
			},
			"acceptance_criteria": []string{
				"all required tests pass",
				"openspec validation passes",
			},
			"non_goals": []string{
				"rewrite entire frontend",
			},
		}
		createBuf, _ := json.Marshal(createPayload)
		createRes, err := http.Post(srv.URL+"/api/mission-contract", "application/json", bytes.NewReader(createBuf))
		if err != nil {
			t.Fatalf("create mission contract: %v", err)
		}
		defer createRes.Body.Close()
		if createRes.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(createRes.Body)
			t.Fatalf("create status=%d, want 200; body=%s", createRes.StatusCode, string(body))
		}
		var createBody struct {
			OK       bool                   `json:"ok"`
			Contract *tasks.MissionContract `json:"contract"`
		}
		if err := json.NewDecoder(createRes.Body).Decode(&createBody); err != nil {
			t.Fatalf("decode create body: %v", err)
		}
		if !createBody.OK || createBody.Contract == nil {
			t.Fatalf("unexpected create response: %+v", createBody)
		}
		if createBody.Contract.Revision != 1 {
			t.Fatalf("create revision=%d, want 1", createBody.Contract.Revision)
		}

		getRes, err := http.Get(srv.URL + "/api/mission-contract?task_id=" + url.QueryEscape(task.ID))
		if err != nil {
			t.Fatalf("get mission contract: %v", err)
		}
		defer getRes.Body.Close()
		if getRes.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(getRes.Body)
			t.Fatalf("get status=%d, want 200; body=%s", getRes.StatusCode, string(body))
		}
		var getBody struct {
			OK       bool                   `json:"ok"`
			Contract *tasks.MissionContract `json:"contract"`
		}
		if err := json.NewDecoder(getRes.Body).Decode(&getBody); err != nil {
			t.Fatalf("decode get body: %v", err)
		}
		if !getBody.OK || getBody.Contract == nil {
			t.Fatalf("unexpected get response: %+v", getBody)
		}
		if getBody.Contract.Goal != "Deliver autonomous execution loop safely" {
			t.Fatalf("get goal=%q", getBody.Contract.Goal)
		}

		updatePayload := map[string]any{
			"key":  tasks.SessionKeyForTask(task),
			"goal": "Deliver autonomous execution loop safely at scale",
			"constraints": []string{
				"always run tests before commit",
			},
			"acceptance_criteria": []string{
				"all required tests pass",
				"no cross-project contamination",
			},
			"non_goals": []string{
				"replace language runtime",
			},
		}
		updateBuf, _ := json.Marshal(updatePayload)
		updateReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/mission-contract", bytes.NewReader(updateBuf))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		updateReq.Header.Set("Content-Type", "application/json")
		updateRes, err := http.DefaultClient.Do(updateReq)
		if err != nil {
			t.Fatalf("update mission contract: %v", err)
		}
		defer updateRes.Body.Close()
		if updateRes.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(updateRes.Body)
			t.Fatalf("update status=%d, want 200; body=%s", updateRes.StatusCode, string(body))
		}
		var updateBody struct {
			OK       bool                   `json:"ok"`
			Contract *tasks.MissionContract `json:"contract"`
		}
		if err := json.NewDecoder(updateRes.Body).Decode(&updateBody); err != nil {
			t.Fatalf("decode update body: %v", err)
		}
		if !updateBody.OK || updateBody.Contract == nil {
			t.Fatalf("unexpected update response: %+v", updateBody)
		}
		if updateBody.Contract.Revision != 2 {
			t.Fatalf("update revision=%d, want 2", updateBody.Contract.Revision)
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

		t.Run("import env", func(t *testing.T) {
			t.Setenv("ANTHROPIC_BASE_URL", "https://anthropic.example.test")
			t.Setenv("ANTHROPIC_API_KEY", "sk-ant-env-supersecret-123456")
			t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
			t.Setenv("ANTHROPIC_MODEL", "claude-test-model")
			t.Setenv("ANTHROPIC_SMALL_FAST_MODEL", "")
			t.Setenv("OPENAI_API_KEY", "sk-openai-env-supersecret-654321")

			store, err := auth.Load(filepath.Join(t.TempDir(), "secrets.json"))
			if err != nil {
				t.Fatalf("auth load: %v", err)
			}
			apiSvc.Auth = store

			buf, _ := json.Marshal(map[string]string{"target": "all"})
			importRes, err := http.Post(srv.URL+"/api/auth/import/env", "application/json", bytes.NewReader(buf))
			if err != nil {
				t.Fatalf("post import: %v", err)
			}
			defer importRes.Body.Close()
			rawBody, _ := io.ReadAll(importRes.Body)
			if importRes.StatusCode != http.StatusOK {
				t.Fatalf("import status=%d, want 200; body=%s", importRes.StatusCode, string(rawBody))
			}
			if strings.Contains(string(rawBody), "sk-ant-env-supersecret-123456") {
				t.Fatalf("expected response to not include raw anthropic secret")
			}
			if strings.Contains(string(rawBody), "sk-openai-env-supersecret-654321") {
				t.Fatalf("expected response to not include raw openai secret")
			}

			var body struct {
				Status      auth.Status `json:"status"`
				StoragePath string      `json:"storage_path"`
				Imported    []string    `json:"imported"`
			}
			if err := json.Unmarshal(rawBody, &body); err != nil {
				t.Fatalf("decode import: %v", err)
			}
			if body.StoragePath != store.Path() {
				t.Fatalf("storage_path=%q, want %q", body.StoragePath, store.Path())
			}
			if body.Status.Claude.BaseURL.Effective != "stored" || body.Status.Claude.APIKey.Effective != "stored" {
				t.Fatalf("expected stored effective status, got claude=%+v", body.Status.Claude)
			}
			if body.Status.Codex.APIKey.Effective != "stored" {
				t.Fatalf("expected stored effective status, got codex=%+v", body.Status.Codex)
			}

			secrets := store.Get()
			if secrets.AnthropicBaseURL != "https://anthropic.example.test" {
				t.Fatalf("anthropic_base_url=%q, want env value", secrets.AnthropicBaseURL)
			}
			if secrets.AnthropicAPIKey != "sk-ant-env-supersecret-123456" {
				t.Fatalf("anthropic_api_key=%q, want env value", secrets.AnthropicAPIKey)
			}
			if secrets.AnthropicModel != "claude-test-model" {
				t.Fatalf("anthropic_model=%q, want env value", secrets.AnthropicModel)
			}
			if secrets.OpenAIAPIKey != "sk-openai-env-supersecret-654321" {
				t.Fatalf("openai_api_key=%q, want env value", secrets.OpenAIAPIKey)
			}
			if len(body.Imported) < 3 {
				t.Fatalf("expected imported fields, got %v", body.Imported)
			}
		})
	})
}
