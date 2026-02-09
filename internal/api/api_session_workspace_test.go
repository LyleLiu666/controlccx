package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/runworkspace"
	"controlccx/internal/tasks"
)

func TestAPI_SessionWorkspace_GetMergeDiscard_CopyMode(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	workspacesSvc := runworkspace.NewService(store, runworkspace.Options{Retain: 5})
	apiSvc := &API{Tasks: store, Workspaces: workspacesSvc}

	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo ok",
		WorkDir:    base,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	key := tasks.SessionKeyForTask(task)

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// No workspace yet.
	{
		res, err := http.Get(srv.URL + "/api/sessions/" + url.PathEscape(key) + "/workspace")
		if err != nil {
			t.Fatalf("get workspace: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var payload struct {
			OK        bool                    `json:"ok"`
			Workspace *tasks.SessionWorkspace `json:"workspace"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.OK || payload.Workspace != nil {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	}

	ens, err := workspacesSvc.EnsureForTask(ctx, task)
	if err != nil {
		t.Fatalf("EnsureForTask: %v", err)
	}

	// Get workspace metadata.
	{
		res, err := http.Get(srv.URL + "/api/sessions/" + url.PathEscape(key) + "/workspace")
		if err != nil {
			t.Fatalf("get workspace: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var payload struct {
			OK        bool                    `json:"ok"`
			Workspace *tasks.SessionWorkspace `json:"workspace"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !payload.OK || payload.Workspace == nil {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if payload.Workspace.Key != key {
			t.Fatalf("workspace key=%q, want %q", payload.Workspace.Key, key)
		}
		if payload.Workspace.Kind != "copy" {
			t.Fatalf("kind=%q, want %q", payload.Workspace.Kind, "copy")
		}
		if payload.Workspace.BaseWorkDir != filepath.Clean(base) {
			t.Fatalf("base=%q, want %q", payload.Workspace.BaseWorkDir, filepath.Clean(base))
		}
		if payload.Workspace.RunWorkDir == "" {
			t.Fatalf("expected run_workdir set")
		}
	}

	// Modify workspace file.
	if err := os.WriteFile(filepath.Join(ens.Workspace.RunRoot, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("write workspace: %v", err)
	}

	// Merge back applies change.
	{
		res, err := http.Post(
			srv.URL+"/api/sessions/"+url.PathEscape(key)+"/workspace/merge",
			"application/json",
			bytes.NewReader([]byte("{}")),
		)
		if err != nil {
			t.Fatalf("post merge: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var out runworkspace.MergeResult
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Workspace.Status != "merged" {
			t.Fatalf("status=%q, want %q", out.Workspace.Status, "merged")
		}
		if len(out.Conflicts) != 0 {
			t.Fatalf("conflicts=%v, want none", out.Conflicts)
		}
	}

	got, err := os.ReadFile(filepath.Join(base, "a.txt"))
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	if string(got) != "two\n" {
		t.Fatalf("base a.txt=%q, want %q", string(got), "two\\n")
	}

	// Discard marks discarded.
	{
		res, err := http.Post(
			srv.URL+"/api/sessions/"+url.PathEscape(key)+"/workspace/discard",
			"application/json",
			bytes.NewReader([]byte("{}")),
		)
		if err != nil {
			t.Fatalf("post discard: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var out struct {
			OK bool `json:"ok"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !out.OK {
			t.Fatalf("expected ok=true")
		}
	}

	ws, ok, err := store.GetSessionWorkspace(ctx, key)
	if err != nil || !ok {
		t.Fatalf("expected workspace record; ok=%v err=%v", ok, err)
	}
	if ws.Status != "discarded" {
		t.Fatalf("status=%q, want %q", ws.Status, "discarded")
	}
}

func TestAPI_SessionWorkspace_Ensure_CreatesWorkspace(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	workspacesSvc := runworkspace.NewService(store, runworkspace.Options{Retain: 5})
	apiSvc := &API{Tasks: store, Workspaces: workspacesSvc}

	base := t.TempDir()
	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "echo ok",
		WorkDir:    base,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	key := tasks.SessionKeyForTask(task)

	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// Ensure workspace.
	{
		res, err := http.Post(
			srv.URL+"/api/sessions/"+url.PathEscape(key)+"/workspace/ensure",
			"application/json",
			bytes.NewReader([]byte("{}")),
		)
		if err != nil {
			t.Fatalf("post ensure: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var payload struct {
			OK        bool                    `json:"ok"`
			Workspace *tasks.SessionWorkspace `json:"workspace"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !payload.OK || payload.Workspace == nil {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if payload.Workspace.Key == "" || payload.Workspace.RunWorkDir == "" {
			t.Fatalf("expected workspace key/run_workdir set: %+v", payload.Workspace)
		}
	}

	// Get workspace metadata should now return ok=true.
	{
		res, err := http.Get(srv.URL + "/api/sessions/" + url.PathEscape(key) + "/workspace")
		if err != nil {
			t.Fatalf("get workspace: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d, want 200", res.StatusCode)
		}
		var payload struct {
			OK        bool                    `json:"ok"`
			Workspace *tasks.SessionWorkspace `json:"workspace"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !payload.OK || payload.Workspace == nil {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	}
}
