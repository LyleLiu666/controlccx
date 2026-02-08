package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/providers"
)

func TestAPI_Providers_UpsertActivateSpeedTest(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// List (empty).
	{
		res, err := http.Get(srv.URL + "/api/providers")
		if err != nil {
			t.Fatalf("get providers: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var body struct {
			Profiles []providers.Profile       `json:"profiles"`
			Active   providers.ActiveSelection `json:"active"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Profiles) != 0 {
			t.Fatalf("expected empty profiles")
		}
		if body.Active.Claude != "" || body.Active.Codex != "" || body.Active.Secretary != "" {
			t.Fatalf("unexpected active: %+v", body.Active)
		}
	}

	// Upsert.
	var created providers.Profile
	rawToken := "sk-ant-test-abcdef"
	{
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{
				Name: "P1",
				Targets: providers.Targets{
					Claude: providers.ClaudeTarget{
						BaseURL:   "https://example.invalid/api/anthropic",
						AuthToken: rawToken,
						Model:     "claude-3-7-sonnet",
					},
				},
			},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("upsert status=%d, want 200", res.StatusCode)
		}
		var body struct {
			Profile providers.Profile `json:"profile"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		created = body.Profile
		if created.ID == "" {
			t.Fatalf("expected id")
		}
		if created.Targets.Claude.AuthToken == rawToken {
			t.Fatalf("expected auth token to be masked in response")
		}
	}

	// Activate.
	{
		reqBody, _ := json.Marshal(map[string]any{
			"target": "claude",
			"id":     created.ID,
		})
		res, err := http.Post(srv.URL+"/api/providers/activate", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post activate: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("activate status=%d, want 200", res.StatusCode)
		}
		if got := providersStore.Active().Claude; got != created.ID {
			t.Fatalf("active claude got=%q want=%q", got, created.ID)
		}
		if got := authStore.Get().AnthropicAuthToken; got != "sk-ant-test-abcdef" {
			t.Fatalf("auth token stored got=%q", got)
		}
	}

	// Speed test (best-effort reachability).
	{
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(backend.Close)

		// Update profile base_url to point to test server.
		created.Targets.Claude.BaseURL = backend.URL
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{
				ID:   created.ID,
				Name: created.Name,
				Targets: providers.Targets{
					Claude: providers.ClaudeTarget{
						BaseURL: backend.URL,
						Model:   created.Targets.Claude.Model,
						// Leave secrets empty; API MUST keep the existing stored secrets.
					},
				},
			},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert2: %v", err)
		}
		res.Body.Close()
		if got := providersStore.Profiles()[0].Targets.Claude.AuthToken; got != rawToken {
			t.Fatalf("expected stored auth token preserved, got=%q", got)
		}

		reqBody, _ = json.Marshal(map[string]any{
			"target":     "claude",
			"id":         created.ID,
			"timeout_ms": 200,
		})
		start := time.Now()
		res, err = http.Post(srv.URL+"/api/providers/speedtest", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post speedtest: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("speedtest status=%d, want 200", res.StatusCode)
		}
		var body struct {
			Result providers.SpeedTestResult `json:"result"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.Result.OK || body.Result.StatusCode != http.StatusNoContent {
			t.Fatalf("unexpected result: %+v", body.Result)
		}
		if time.Since(start) > 500*time.Millisecond {
			t.Fatalf("expected speedtest to respect timeout, took %v", time.Since(start))
		}
	}
}

func TestAPI_Providers_Upsert_PreservesSecretarySimpleHTTPSecrets(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	rawToken := "sec-token-abc123"

	// Create.
	var created providers.Profile
	{
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{
				Name: "P1",
				Targets: providers.Targets{
					Secretary: providers.SecretaryTarget{
						Backend: "simple-http",
						SimpleHTTP: providers.SecretarySimpleHTTP{
							BaseURL:   "https://example.invalid/api/anthropic",
							AuthToken: rawToken,
							Model:     "claude-test",
						},
					},
				},
			},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("upsert status=%d, want 200", res.StatusCode)
		}
		var body struct {
			Profile providers.Profile `json:"profile"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		created = body.Profile
		if created.ID == "" {
			t.Fatalf("expected id")
		}
		if created.Targets.Secretary.SimpleHTTP.AuthToken == rawToken {
			t.Fatalf("expected secretary auth token to be masked in response")
		}
	}

	// Update base_url, leave secret empty -> MUST preserve stored secret.
	{
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{
				ID:   created.ID,
				Name: created.Name,
				Targets: providers.Targets{
					Secretary: providers.SecretaryTarget{
						Backend: "simple-http",
						SimpleHTTP: providers.SecretarySimpleHTTP{
							BaseURL: "https://example.invalid/changed",
							Model:   "claude-test",
							// Leave secrets empty; API MUST keep the existing stored secrets.
						},
					},
				},
			},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert2: %v", err)
		}
		res.Body.Close()

		got := providersStore.Profiles()[0].Targets.Secretary.SimpleHTTP.AuthToken
		if got != rawToken {
			t.Fatalf("expected stored auth token preserved, got=%q", got)
		}
	}
}

func TestAPI_Providers_Upsert_RejectsDuplicateName(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	create := func(name string) int {
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{Name: name},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert(%s): %v", name, err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	if status := create("Current"); status != http.StatusOK {
		t.Fatalf("first upsert status=%d, want 200", status)
	}
	if status := create("current"); status != http.StatusBadRequest {
		t.Fatalf("duplicate upsert status=%d, want 400", status)
	}
}

func TestAPI_Providers_Import_MergeAndRenameOnDuplicateNames(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	_, err = providersStore.Upsert(providers.Profile{
		Name: "Current",
		Tool: "claude",
		Targets: providers.Targets{
			Claude: providers.ClaudeTarget{
				BaseURL: "https://example.invalid",
				Model:   "claude-3-7-sonnet",
			},
		},
	})
	if err != nil {
		t.Fatalf("seed existing profile: %v", err)
	}

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	reqBody, _ := json.Marshal(map[string]any{
		"profiles": []providers.Profile{
			{
				ID:   "import-fixed-id-1",
				Name: "Current",
				Tool: "claude",
				Targets: providers.Targets{
					Claude: providers.ClaudeTarget{
						BaseURL:   "https://api.anthropic.com",
						AuthToken: "sk-ant-import-123456",
						Model:     "claude-import-a",
					},
				},
			},
			{
				ID:   "import-fixed-id-2",
				Name: "Current",
				Tool: "codex",
				Targets: providers.Targets{
					Codex: providers.CodexTarget{
						BaseURL: "https://api.openai.com",
						APIKey:  "sk-openai-import-123456",
						Model:   "gpt-5",
					},
				},
			},
		},
	})
	res, err := http.Post(srv.URL+"/api/providers/import", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post providers import: %v", err)
	}
	defer res.Body.Close()
	rawBody, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("providers import status=%d, want 200; body=%s", res.StatusCode, string(rawBody))
	}

	var body struct {
		Imported []providers.Profile `json:"imported"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode providers import: %v", err)
	}
	if len(body.Imported) != 2 {
		t.Fatalf("imported len=%d, want 2", len(body.Imported))
	}

	if got := body.Imported[0].Name; got != "Current (导入)" {
		t.Fatalf("first imported name=%q, want %q", got, "Current (导入)")
	}
	if got := body.Imported[1].Name; got != "Current (导入 2)" {
		t.Fatalf("second imported name=%q, want %q", got, "Current (导入 2)")
	}
	if got := strings.TrimSpace(body.Imported[0].ID); got == "" || got == "import-fixed-id-1" {
		t.Fatalf("first imported id=%q, expected new generated id", got)
	}
	if got := strings.TrimSpace(body.Imported[1].ID); got == "" || got == "import-fixed-id-2" {
		t.Fatalf("second imported id=%q, expected new generated id", got)
	}
	if got := body.Imported[0].Targets.Claude.AuthToken; got == "sk-ant-import-123456" {
		t.Fatalf("expected imported auth token masked in response")
	}
	if got := body.Imported[1].Targets.Codex.APIKey; got == "sk-openai-import-123456" {
		t.Fatalf("expected imported api key masked in response")
	}

	profiles := providersStore.Profiles()
	if len(profiles) != 3 {
		t.Fatalf("store profiles len=%d, want 3", len(profiles))
	}
}

func TestAPI_Providers_ImportEnv(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	t.Setenv("ANTHROPIC_BASE_URL", "https://env.anthropic.example")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-ant-env-import-123456")
	t.Setenv("ANTHROPIC_MODEL", "claude-env-model")
	t.Setenv("OPENAI_API_KEY", "sk-openai-env-import-123456")

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	reqBody, _ := json.Marshal(map[string]any{"target": "claude"})
	res, err := http.Post(srv.URL+"/api/providers/import/env", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post import env: %v", err)
	}
	defer res.Body.Close()
	rawBody, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import env status=%d, want 200; body=%s", res.StatusCode, string(rawBody))
	}

	var body struct {
		Profile providers.Profile `json:"profile"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode import env: %v", err)
	}
	if got := body.Profile.Targets.Claude.BaseURL; got != "https://env.anthropic.example" {
		t.Fatalf("claude.base_url=%q, want env value", got)
	}
	if got := body.Profile.Targets.Claude.AuthToken; got != "sk-ant-env-import-123456" {
		t.Fatalf("claude.auth_token=%q, want env value", got)
	}
	if got := body.Profile.Targets.Claude.Model; got != "claude-env-model" {
		t.Fatalf("claude.model=%q, want env value", got)
	}
}

func TestAPI_Providers_ImportEnv_CodexFromHomeFiles(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("OPENAI_API_KEY", "sk-openai-env-should-not-be-used")
	t.Setenv("CODEX_MODEL", "env-codex-model-should-not-be-used")
	t.Setenv("CODEX_REASONING_EFFORT", "high")

	codexHome := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-openai-file-123456"}`), 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("model = \"gpt-5.2\"\nmodel_reasoning_effort = \"medium\"\n"), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	reqBody, _ := json.Marshal(map[string]any{"target": "codex"})
	res, err := http.Post(srv.URL+"/api/providers/import/env", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post import env codex: %v", err)
	}
	defer res.Body.Close()
	rawBody, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("import env codex status=%d, want 200; body=%s", res.StatusCode, string(rawBody))
	}

	var body struct {
		Profile  providers.Profile `json:"profile"`
		Imported []string          `json:"imported"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode import env codex: %v", err)
	}
	if got := body.Profile.Tool; got != "codex" {
		t.Fatalf("profile.tool=%q, want codex", got)
	}
	if got := body.Profile.Targets.Codex.APIKey; got != "sk-openai-file-123456" {
		t.Fatalf("codex.api_key=%q, want file value", got)
	}
	if got := body.Profile.Targets.Codex.Model; got != "gpt-5.2" {
		t.Fatalf("codex.model=%q, want file value", got)
	}
	if got := body.Profile.Targets.Codex.ReasoningEffort; got != "medium" {
		t.Fatalf("codex.reasoning_effort=%q, want file value", got)
	}
	if got := body.Profile.Targets.Codex.APIKey; got == "sk-openai-env-should-not-be-used" {
		t.Fatalf("codex.api_key unexpectedly imported from env")
	}
	if len(body.Imported) == 0 {
		t.Fatalf("imported should include codex file-derived fields")
	}
}

func TestAPI_Providers_Ping_SecretarySimpleHTTP(t *testing.T) {
	dataDir := t.TempDir()
	providersStore, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("providers.NewStore: %v", err)
	}
	authStore, err := auth.Load(filepath.Join(dataDir, "secrets.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}

	wantToken := "sec-token-abc123"
	authzCh := make(chan string, 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		select {
		case authzCh <- r.Header.Get("Authorization"):
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
	}))
	t.Cleanup(backend.Close)

	apiSvc := &API{Providers: providersStore, Auth: authStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// Create provider with stored secretary simple-http secrets.
	var created providers.Profile
	{
		reqBody, _ := json.Marshal(map[string]any{
			"profile": providers.Profile{
				Name: "Sec1",
				Targets: providers.Targets{
					Secretary: providers.SecretaryTarget{
						Backend: "simple-http",
						SimpleHTTP: providers.SecretarySimpleHTTP{
							BaseURL:   backend.URL,
							AuthToken: wantToken,
							Model:     "claude-test",
						},
					},
				},
			},
		})
		res, err := http.Post(srv.URL+"/api/providers/upsert", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post upsert: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("upsert status=%d, want 200", res.StatusCode)
		}
		var body struct {
			Profile providers.Profile `json:"profile"`
		}
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		created = body.Profile
		if created.ID == "" {
			t.Fatalf("expected id")
		}
	}

	// Ping test: omit secrets (editor keeps them blank); API MUST use stored secrets.
	{
		reqBody, _ := json.Marshal(map[string]any{
			"id":         created.ID,
			"base_url":   backend.URL,
			"model":      "claude-test",
			"timeout_ms": 1000,
		})
		res, err := http.Post(srv.URL+"/api/providers/ping", "application/json", bytes.NewReader(reqBody))
		if err != nil {
			t.Fatalf("post ping: %v", err)
		}
		defer res.Body.Close()
		rawBody, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("ping status=%d, want 200; body=%s", res.StatusCode, string(rawBody))
		}
		var body struct {
			Result providers.PingTestResult `json:"result"`
		}
		if err := json.Unmarshal(rawBody, &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !body.Result.OK {
			t.Fatalf("expected ok, got=%+v", body.Result)
		}
		if strings.TrimSpace(body.Result.Response) == "" {
			t.Fatalf("expected response")
		}
		if strings.ToLower(strings.TrimSpace(body.Result.Response)) != "pong" {
			t.Fatalf("response=%q", body.Result.Response)
		}
	}

	select {
	case got := <-authzCh:
		if got != "Bearer "+wantToken {
			t.Fatalf("Authorization=%q", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected backend request")
	}
}
