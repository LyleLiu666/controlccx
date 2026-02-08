package chat

import (
	"context"
	"path/filepath"
	"testing"

	"controlccx/internal/db"
)

func TestStore_ClearAndPruneKeepLast(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	_, _ = store.Append(ctx, RoleUser, "m1")
	_, _ = store.Append(ctx, RoleAssistant, "m2")
	_, _ = store.Append(ctx, RoleUser, "m3")

	if err := store.PruneKeepLast(ctx, 2); err != nil {
		t.Fatalf("prune: %v", err)
	}

	msgs, err := store.Tail(ctx, 10)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len=%d want %d", len(msgs), 2)
	}
	if msgs[0].Content != "m2" || msgs[1].Content != "m3" {
		t.Fatalf("contents=%q,%q want %q,%q", msgs[0].Content, msgs[1].Content, "m2", "m3")
	}

	if err := store.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	msgs, err = store.Tail(ctx, 10)
	if err != nil {
		t.Fatalf("tail after clear: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("len=%d want %d", len(msgs), 0)
	}
}
