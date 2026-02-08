package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/audit"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestAPI_AuditEndpoints(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	base := time.Date(2026, 2, 8, 20, 0, 0, 0, time.UTC)
	if _, err := conn.ExecContext(ctx, `INSERT INTO tasks(id, worker_type, mode, status, prompt, workdir, session_id, conversation_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		"task-a", "exec", "new", "succeeded", "seed", "/tmp", "", "conv-task-a", toMsAPI(base), toMsAPI(base)); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO logs(task_id, ts, stream, message) VALUES (?, ?, ?, ?);`, "task-a", toMsAPI(base), "system", "safety.autopilot blocked"); err != nil {
		t.Fatalf("insert log: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO secretary_events(ts, run_id, kind, protocol, step, event_json) VALUES (?, ?, ?, ?, ?, ?);`, toMsAPI(base.Add(time.Minute)), "run-a", "trace", "xml", 1, `{"message":"hello"}`); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	apiSvc := &API{
		Tasks: tasks.NewStore(conn),
		Audit: audit.NewService(conn, audit.Options{}),
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/audit/entries?limit=10&q=blocked")
	if err != nil {
		t.Fatalf("get entries: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("entries status=%d want 200", res.StatusCode)
	}
	var out struct {
		Entries    []audit.Entry `json:"entries"`
		NextCursor string        `json:"next_cursor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(out.Entries) == 0 {
		t.Fatalf("expected entries")
	}
	if out.Entries[0].ID == "" {
		t.Fatalf("expected non-empty entry id")
	}

	detailRes, err := http.Get(srv.URL + "/api/audit/entries/" + out.Entries[0].ID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	defer detailRes.Body.Close()
	if detailRes.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d want 200", detailRes.StatusCode)
	}
	var detail audit.EntryDetail
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != out.Entries[0].ID {
		t.Fatalf("detail id=%q want %q", detail.ID, out.Entries[0].ID)
	}

	sourcesRes, err := http.Get(srv.URL + "/api/audit/sources")
	if err != nil {
		t.Fatalf("get sources: %v", err)
	}
	defer sourcesRes.Body.Close()
	if sourcesRes.StatusCode != http.StatusOK {
		t.Fatalf("sources status=%d want 200", sourcesRes.StatusCode)
	}

	retRes, err := http.Get(srv.URL + "/api/audit/retention")
	if err != nil {
		t.Fatalf("get retention: %v", err)
	}
	defer retRes.Body.Close()
	if retRes.StatusCode != http.StatusOK {
		t.Fatalf("retention status=%d want 200", retRes.StatusCode)
	}
}

func TestAPI_AuditLoopbackOnly(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	h := (&API{Tasks: tasks.NewStore(conn), Audit: audit.NewService(conn, audit.Options{})}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/entries", nil)
	req.RemoteAddr = "8.8.8.8:34567"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("body=%q want forbidden", rr.Body.String())
	}
}

func toMsAPI(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}
