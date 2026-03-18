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

func TestStore_ConversationPartitionOps(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)

	_, _ = store.AppendInConversation(ctx, "conv-a", RoleUser, "a1")
	_, _ = store.AppendInConversation(ctx, "conv-a", RoleAssistant, "a2")
	_, _ = store.AppendInConversation(ctx, "conv-a", RoleUser, "a3")

	_, _ = store.AppendInConversation(ctx, "conv-b", RoleUser, "b1")
	_, _ = store.AppendInConversation(ctx, "conv-b", RoleAssistant, "b2")

	convA, err := store.TailInConversation(ctx, "conv-a", 10)
	if err != nil {
		t.Fatalf("tail conv-a: %v", err)
	}
	if len(convA) != 3 {
		t.Fatalf("conv-a len=%d want 3", len(convA))
	}
	if convA[0].Content != "a1" || convA[1].Content != "a2" || convA[2].Content != "a3" {
		t.Fatalf("unexpected conv-a history: %+v", convA)
	}

	convB, err := store.TailInConversation(ctx, "conv-b", 10)
	if err != nil {
		t.Fatalf("tail conv-b: %v", err)
	}
	if len(convB) != 2 {
		t.Fatalf("conv-b len=%d want 2", len(convB))
	}
	if convB[0].Content != "b1" || convB[1].Content != "b2" {
		t.Fatalf("unexpected conv-b history: %+v", convB)
	}

	if err := store.PruneKeepLastInConversation(ctx, "conv-a", 2); err != nil {
		t.Fatalf("prune conv-a: %v", err)
	}
	convA, err = store.TailInConversation(ctx, "conv-a", 10)
	if err != nil {
		t.Fatalf("tail conv-a after prune: %v", err)
	}
	if len(convA) != 2 {
		t.Fatalf("conv-a len after prune=%d want 2", len(convA))
	}
	if convA[0].Content != "a2" || convA[1].Content != "a3" {
		t.Fatalf("unexpected conv-a after prune: %+v", convA)
	}

	convB, err = store.TailInConversation(ctx, "conv-b", 10)
	if err != nil {
		t.Fatalf("tail conv-b after conv-a prune: %v", err)
	}
	if len(convB) != 2 {
		t.Fatalf("conv-b len after conv-a prune=%d want 2", len(convB))
	}

	if err := store.ClearConversation(ctx, "conv-a"); err != nil {
		t.Fatalf("clear conv-a: %v", err)
	}
	convA, err = store.TailInConversation(ctx, "conv-a", 10)
	if err != nil {
		t.Fatalf("tail conv-a after clear: %v", err)
	}
	if len(convA) != 0 {
		t.Fatalf("conv-a len after clear=%d want 0", len(convA))
	}

	convB, err = store.TailInConversation(ctx, "conv-b", 10)
	if err != nil {
		t.Fatalf("tail conv-b after conv-a clear: %v", err)
	}
	if len(convB) != 2 {
		t.Fatalf("conv-b len after conv-a clear=%d want 2", len(convB))
	}
}
