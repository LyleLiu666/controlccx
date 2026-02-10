package tasks

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"controlccx/internal/db"
)

func TestStore_SessionContinueQueue_OrderByPriorityThenFIFO(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "c-1"

	rawB, _ := json.Marshal(map[string]any{"prompt": "B"})
	rawC, _ := json.Marshal(map[string]any{"prompt": "C"})
	rawD, _ := json.Marshal(map[string]any{"prompt": "D"})

	b, err := store.EnqueueSessionContinue(ctx, EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         "B",
		RunOptionsJSON: string(rawB),
		Priority:       0,
		Source:         "continue",
	})
	if err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	c, err := store.EnqueueSessionContinue(ctx, EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         "C",
		RunOptionsJSON: string(rawC),
		Priority:       100,
		Source:         "preempt",
	})
	if err != nil {
		t.Fatalf("enqueue C: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	d, err := store.EnqueueSessionContinue(ctx, EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         "D",
		RunOptionsJSON: string(rawD),
		Priority:       0,
		Source:         "continue",
	})
	if err != nil {
		t.Fatalf("enqueue D: %v", err)
	}

	list, err := store.ListSessionContinueQueueByConversation(ctx, conversationID, 10)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len=%d, want 3", len(list))
	}
	if list[0].ID != c.ID {
		t.Fatalf("queue[0]=%s, want %s (C first)", list[0].ID, c.ID)
	}
	if list[1].ID != b.ID {
		t.Fatalf("queue[1]=%s, want %s (B second)", list[1].ID, b.ID)
	}
	if list[2].ID != d.ID {
		t.Fatalf("queue[2]=%s, want %s (D third)", list[2].ID, d.ID)
	}
}

func TestStore_SessionContinueQueue_ClaimAndMarkDone(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewStore(conn)
	conversationID := "c-claim"

	if _, err := store.EnqueueSessionContinue(ctx, EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         "B",
		RunOptionsJSON: `{"prompt":"B"}`,
		Priority:       0,
		Source:         "continue",
	}); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	if _, err := store.EnqueueSessionContinue(ctx, EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         "C",
		RunOptionsJSON: `{"prompt":"C"}`,
		Priority:       100,
		Source:         "preempt",
	}); err != nil {
		t.Fatalf("enqueue C: %v", err)
	}

	next, ok, err := store.ClaimNextSessionContinue(ctx, conversationID)
	if err != nil {
		t.Fatalf("claim #1: %v", err)
	}
	if !ok {
		t.Fatalf("claim #1: ok=false, want true")
	}
	if next.Prompt != "C" {
		t.Fatalf("claim #1 prompt=%q, want C", next.Prompt)
	}
	if next.State != SessionContinueQueueStateDispatching {
		t.Fatalf("claim #1 state=%q, want dispatching", next.State)
	}
	if err := store.MarkSessionContinueQueueState(ctx, next.ID, SessionContinueQueueStateDone); err != nil {
		t.Fatalf("mark done: %v", err)
	}

	next2, ok, err := store.ClaimNextSessionContinue(ctx, conversationID)
	if err != nil {
		t.Fatalf("claim #2: %v", err)
	}
	if !ok {
		t.Fatalf("claim #2: ok=false, want true")
	}
	if next2.Prompt != "B" {
		t.Fatalf("claim #2 prompt=%q, want B", next2.Prompt)
	}
}
