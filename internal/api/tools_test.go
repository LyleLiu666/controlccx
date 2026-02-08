package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"controlccx/internal/tooling"
)

func TestAPI_Tools_ListUpsertDelete(t *testing.T) {
	dataDir := t.TempDir()
	svc, err := tooling.NewService(tooling.Options{
		DataDir:  dataDir,
		Defaults: []tooling.Tool{{ID: "claude-code", Driver: tooling.DriverClaudeCode, Command: "claude"}, {ID: "codex", Driver: tooling.DriverCodex, Command: "codex"}},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	apiSvc := &API{Tools: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/tools")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}
	var list struct {
		Tools []tooling.Tool `json:"tools"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Tools) != 2 || list.Tools[0].ID != "claude-code" {
		t.Fatalf("tools=%v", list.Tools)
	}

	upsertBody, _ := json.Marshal(map[string]any{
		"tool": tooling.Tool{
			ID:      "claude-code",
			Driver:  tooling.DriverClaudeCode,
			Command: "/x/claude",
			Env:     map[string]string{"CONTROLCCX_TEST_ENV": "1"},
		},
	})
	upsertRes, err := http.Post(srv.URL+"/api/tools/upsert", "application/json", bytes.NewReader(upsertBody))
	if err != nil {
		t.Fatalf("post upsert: %v", err)
	}
	defer upsertRes.Body.Close()
	if upsertRes.StatusCode != http.StatusOK {
		t.Fatalf("upsert status=%d, want 200", upsertRes.StatusCode)
	}

	if got, ok := svc.Resolve("claude-code"); !ok || got.Command != "/x/claude" {
		t.Fatalf("expected tool override to be upserted, ok=%v tool=%#v", ok, got)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "tools.json")); err != nil {
		t.Fatalf("expected tools.json to exist: %v", err)
	}

	deleteBody, _ := json.Marshal(map[string]any{"id": "claude-code"})
	deleteRes, err := http.Post(srv.URL+"/api/tools/delete", "application/json", bytes.NewReader(deleteBody))
	if err != nil {
		t.Fatalf("post delete: %v", err)
	}
	defer deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusOK {
		t.Fatalf("delete status=%d, want 200", deleteRes.StatusCode)
	}

	if got, ok := svc.Resolve("claude-code"); !ok || got.Command != "claude" {
		t.Fatalf("expected tool override to be deleted, ok=%v tool=%#v", ok, got)
	}
}
