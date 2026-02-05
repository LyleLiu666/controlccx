package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_ProjectContext_GetAndSet(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// GET returns empty by default.
	res, err := http.Get(srv.URL + "/api/context")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res.StatusCode)
	}

	// POST sets content.
	setBody := map[string]any{"content": "hello"}
	buf, _ := json.Marshal(setBody)
	postRes, err := http.Post(srv.URL+"/api/context", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	postRes.Body.Close()
	if postRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", postRes.StatusCode)
	}

	// GET returns content.
	res2, err := http.Get(srv.URL + "/api/context")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", res2.StatusCode)
	}
	var got tasks.ProjectContext
	if err := json.NewDecoder(res2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Content != "hello" {
		t.Fatalf("content=%q, want %q", got.Content, "hello")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at set")
	}
}

func TestAPI_PromptTemplates_CRUD(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	apiSvc := &API{Tasks: taskStore}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// Create
	createBody := map[string]any{"title": "t1", "kind": "task", "content": "do it"}
	buf, _ := json.Marshal(createBody)
	createRes, err := http.Post(srv.URL+"/api/templates/upsert", "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", createRes.StatusCode)
	}
	var created struct {
		Template tasks.PromptTemplate `json:"template"`
	}
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Template.ID == "" || created.Template.Kind != "task" {
		t.Fatalf("unexpected template: %+v", created.Template)
	}

	// List
	listRes, err := http.Get(srv.URL + "/api/templates?kind=task")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer listRes.Body.Close()
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", listRes.StatusCode)
	}
	var listed struct {
		Templates []tasks.PromptTemplate `json:"templates"`
	}
	if err := json.NewDecoder(listRes.Body).Decode(&listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed.Templates) != 1 || listed.Templates[0].ID != created.Template.ID {
		t.Fatalf("listed=%+v, want created template", listed.Templates)
	}

	// Delete
	delBody := map[string]any{"id": created.Template.ID}
	delBuf, _ := json.Marshal(delBody)
	delRes, err := http.Post(srv.URL+"/api/templates/delete", "application/json", bytes.NewReader(delBuf))
	if err != nil {
		t.Fatalf("post delete: %v", err)
	}
	delRes.Body.Close()
	if delRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", delRes.StatusCode)
	}

	listRes2, err := http.Get(srv.URL + "/api/templates?kind=task")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer listRes2.Body.Close()
	if listRes2.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", listRes2.StatusCode)
	}
	var listed2 struct {
		Templates []tasks.PromptTemplate `json:"templates"`
	}
	if err := json.NewDecoder(listRes2.Body).Decode(&listed2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(listed2.Templates) != 0 {
		t.Fatalf("expected empty list, got %+v", listed2.Templates)
	}
}
