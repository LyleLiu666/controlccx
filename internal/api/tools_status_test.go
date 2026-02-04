package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"controlccx/internal/tooling"
)

func TestAPI_Tools_Status(t *testing.T) {
	tmp := t.TempDir()

	cmdName := "ccx_fake_tool_bin_9f3d2f"
	createdName := cmdName
	if runtime.GOOS == "windows" {
		createdName = cmdName + ".cmd"
	}

	bin := filepath.Join(tmp, createdName)
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(bin, []byte("@echo off\r\necho ok\r\n"), 0o755); err != nil {
			t.Fatalf("write fake cmd: %v", err)
		}
	} else {
		if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
			t.Fatalf("write fake sh: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})
	if err := os.Setenv("PATH", tmp+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}

	svc, err := tooling.NewService(tooling.Options{
		DataDir: tmp,
		Defaults: []tooling.Tool{
			{ID: "claude-code", Driver: tooling.DriverClaudeCode, Command: cmdName},
			{ID: "codex", Driver: tooling.DriverCodex, Command: "ccx_missing_tool_bin_2b70cbe7a83b"},
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	apiSvc := &API{Tools: svc}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/tools/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	var body struct {
		Tools []struct {
			ID           string `json:"id"`
			Available    bool   `json:"available"`
			ResolvedPath string `json:"resolved_path"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	byID := map[string]struct {
		Available    bool
		ResolvedPath string
	}{}
	for _, ttool := range body.Tools {
		byID[ttool.ID] = struct {
			Available    bool
			ResolvedPath string
		}{Available: ttool.Available, ResolvedPath: ttool.ResolvedPath}
	}

	claude, ok := byID["claude-code"]
	if !ok {
		t.Fatalf("missing claude-code status: %#v", body.Tools)
	}
	if !claude.Available {
		t.Fatalf("claude-code available=false, want true")
	}
	if claude.ResolvedPath == "" {
		t.Fatalf("claude-code resolved_path empty, want non-empty")
	}

	codex, ok := byID["codex"]
	if !ok {
		t.Fatalf("missing codex status: %#v", body.Tools)
	}
	if codex.Available {
		t.Fatalf("codex available=true, want false")
	}
}

