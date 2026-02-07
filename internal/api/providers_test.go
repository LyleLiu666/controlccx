package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
