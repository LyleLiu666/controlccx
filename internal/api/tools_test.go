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
		Defaults: []tooling.Tool{{ID: "claude-code", Driver: tooling.DriverClaudeCode, Command: "claude"}},
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
	if len(list.Tools) != 1 || list.Tools[0].ID != "claude-code" {
		t.Fatalf("tools=%v", list.Tools)
	}

	upsertBody, _ := json.Marshal(map[string]any{
		"tool": tooling.Tool{
			ID:      "claude-cn",
			Driver:  tooling.DriverClaudeCode,
			Command: "claude",
			Env:     map[string]string{"ANTHROPIC_BASE_URL": "https://example.invalid"},
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

	if _, ok := svc.Resolve("claude-cn"); !ok {
		t.Fatalf("expected tool to be upserted")
	}

	if _, err := os.Stat(filepath.Join(dataDir, "tools.json")); err != nil {
		t.Fatalf("expected tools.json to exist: %v", err)
	}

	deleteBody, _ := json.Marshal(map[string]any{"id": "claude-cn"})
	deleteRes, err := http.Post(srv.URL+"/api/tools/delete", "application/json", bytes.NewReader(deleteBody))
	if err != nil {
		t.Fatalf("post delete: %v", err)
	}
	defer deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusOK {
		t.Fatalf("delete status=%d, want 200", deleteRes.StatusCode)
	}

	if _, ok := svc.Resolve("claude-cn"); ok {
		t.Fatalf("expected tool to be deleted")
	}
}
