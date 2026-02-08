package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_ChatEndpointRemoved(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	apiSvc := &API{Tasks: tasks.NewStore(conn)}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/chat")
	if err != nil {
		t.Fatalf("get /api/chat: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want %d", res.StatusCode, http.StatusNotFound)
	}
}
