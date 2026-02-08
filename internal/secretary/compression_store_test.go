package secretary

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestCompressionStore_AppendAndLatest(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewCompressionStore(conn)

	rec, ok, err := store.Latest(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if ok {
		t.Fatalf("expected no latest record, got %+v", rec)
	}

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)
	got, err := store.Append(ctx, CompressionRecord{
		Time:         now,
		CursorBefore: 0,
		CursorAfter:  10,
		KeepFrom:     11,
		Summary:      "  hello  ",
		Backend:      "  claude  ",
		Error:        "",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if got.ID == 0 {
		t.Fatalf("expected id to be set")
	}
	if got.Time.IsZero() {
		t.Fatalf("expected time to be set")
	}
	if got.CursorAfter != 10 || got.KeepFrom != 11 {
		t.Fatalf("unexpected cursors: %+v", got)
	}
	if got.Summary != "hello" {
		t.Fatalf("summary=%q", got.Summary)
	}
	if got.Backend != "claude" {
		t.Fatalf("backend=%q", got.Backend)
	}
	if strings.TrimSpace(got.Error) != "" {
		t.Fatalf("error=%q", got.Error)
	}

	latest, ok, err := store.Latest(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if !ok {
		t.Fatalf("expected latest record")
	}
	if latest.ID != got.ID {
		t.Fatalf("latest id=%d want %d", latest.ID, got.ID)
	}
	if latest.Summary != "hello" {
		t.Fatalf("latest summary=%q", latest.Summary)
	}
}
