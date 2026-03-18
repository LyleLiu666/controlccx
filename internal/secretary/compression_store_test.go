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

func TestCompressionStore_ConversationPartition(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewCompressionStore(conn)

	if _, err := store.AppendInConversation(ctx, "conv-a", CompressionRecord{
		CursorBefore: 0,
		CursorAfter:  10,
		KeepFrom:     11,
		Summary:      "sum-a",
		Backend:      "claude",
	}); err != nil {
		t.Fatalf("append conv-a: %v", err)
	}
	if _, err := store.AppendInConversation(ctx, "conv-b", CompressionRecord{
		CursorBefore: 0,
		CursorAfter:  4,
		KeepFrom:     5,
		Summary:      "sum-b",
		Backend:      "claude",
	}); err != nil {
		t.Fatalf("append conv-b: %v", err)
	}

	a, ok, err := store.LatestInConversation(ctx, "conv-a")
	if err != nil {
		t.Fatalf("latest conv-a: %v", err)
	}
	if !ok {
		t.Fatalf("expected latest for conv-a")
	}
	if a.Summary != "sum-a" {
		t.Fatalf("conv-a summary=%q want %q", a.Summary, "sum-a")
	}

	b, ok, err := store.LatestInConversation(ctx, "conv-b")
	if err != nil {
		t.Fatalf("latest conv-b: %v", err)
	}
	if !ok {
		t.Fatalf("expected latest for conv-b")
	}
	if b.Summary != "sum-b" {
		t.Fatalf("conv-b summary=%q want %q", b.Summary, "sum-b")
	}
}
