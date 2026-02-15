package secretary

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/db"
)

func TestEventStore_AppendAndTail(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewEventStore(conn)
	ev := agentsdk.Event{
		Kind:     agentsdk.EventKindTrace,
		Protocol: "xml",
		Step:     1,
		Time:     time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
		Payload:  agentsdk.TraceEvent{Message: "hello"},
	}
	if err := store.Append(ctx, "run-1", ev); err != nil {
		t.Fatalf("append: %v", err)
	}

	tail, err := store.Tail(ctx, 10)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail) != 1 {
		t.Fatalf("tail len=%d want 1", len(tail))
	}
	if strings.TrimSpace(tail[0].RunID) != "run-1" {
		t.Fatalf("run_id=%q", tail[0].RunID)
	}
	if tail[0].Kind != agentsdk.EventKindTrace {
		t.Fatalf("kind=%q", tail[0].Kind)
	}
	if tail[0].Step != 1 {
		t.Fatalf("step=%d", tail[0].Step)
	}
	if tail[0].Protocol != "xml" {
		t.Fatalf("protocol=%q", tail[0].Protocol)
	}
	if strings.TrimSpace(tail[0].EventJSON) == "" {
		t.Fatalf("expected non-empty event_json")
	}
	var out any
	if err := json.Unmarshal([]byte(tail[0].EventJSON), &out); err != nil {
		t.Fatalf("unmarshal event_json: %v", err)
	}
}

func TestEventStore_PruneKeepLastRuns(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewEventStore(conn)
	for i := 1; i <= 3; i++ {
		runID := "run-" + strconv.Itoa(i)
		if err := store.Append(ctx, runID, agentsdk.Event{
			Kind:     agentsdk.EventKindTrace,
			Protocol: "xml",
			Step:     i,
			Time:     time.Date(2026, 2, 8, 0, 0, 0, i, time.UTC),
			Payload:  agentsdk.TraceEvent{Message: runID},
		}); err != nil {
			t.Fatalf("append %s: %v", runID, err)
		}
	}

	if err := store.PruneKeepLastRuns(ctx, 2); err != nil {
		t.Fatalf("prune: %v", err)
	}

	tail, err := store.Tail(ctx, 100)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	runIDs := map[string]bool{}
	for _, ev := range tail {
		runIDs[ev.RunID] = true
	}
	if runIDs["run-1"] {
		t.Fatalf("expected run-1 pruned, got runIDs=%v", runIDs)
	}
	if !runIDs["run-2"] || !runIDs["run-3"] {
		t.Fatalf("expected run-2 and run-3 kept, got runIDs=%v", runIDs)
	}
}

func TestEventStore_LLMRequest_DoesNotTruncatePromptMessages(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := NewEventStore(conn)

	long := strings.Repeat("x", 4500)
	if strings.Contains(long, "…") {
		t.Fatalf("test setup: expected long string to not contain ellipsis")
	}

	ev := agentsdk.Event{
		Kind:     agentsdk.EventKindLLMRequest,
		Protocol: "xml",
		Step:     1,
		Time:     time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
		Payload: agentsdk.LLMRequestEvent{
			Messages: []agentsdk.Message{{
				Role:    "user",
				Content: long,
			}},
		},
	}
	if err := store.Append(ctx, "run-1", ev); err != nil {
		t.Fatalf("append: %v", err)
	}

	tail, err := store.Tail(ctx, 10)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail) != 1 {
		t.Fatalf("tail len=%d want 1", len(tail))
	}

	var stored struct {
		Payload struct {
			Messages []struct {
				Content string `json:"Content"`
			} `json:"Messages"`
		} `json:"Payload"`
	}
	if err := json.Unmarshal([]byte(tail[0].EventJSON), &stored); err != nil {
		t.Fatalf("unmarshal event_json: %v", err)
	}
	if len(stored.Payload.Messages) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(stored.Payload.Messages))
	}
	got := stored.Payload.Messages[0].Content
	if got != long {
		t.Fatalf("expected stored prompt to be full-length (%d), got %d", len(long), len(got))
	}
	if strings.HasSuffix(got, "…") {
		t.Fatalf("expected stored prompt to not end with ellipsis")
	}
}
