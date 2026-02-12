package secretary

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

func TestService_StartTaskStatusReporter_AutoReportsTerminalAndBlocked(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil)

	hub := events.NewHub()
	stop := svc.StartTaskStatusReporter(context.Background(), hub)
	defer stop()

	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         "task-running",
			Status:     tasks.StatusRunning,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "run smoke",
		},
	})
	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         "task-succeeded",
			Status:     tasks.StatusSucceeded,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "fix tests",
		},
	})
	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: map[string]any{
			"id":          "task-blocked",
			"status":      "blocked",
			"worker_type": "claude-code",
			"prompt":      "deploy release",
			"error":       "approval required",
		},
	})
	// Same status update for same task should not generate duplicate report.
	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         "task-succeeded",
			Status:     tasks.StatusSucceeded,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "fix tests",
		},
	})

	msgs := waitForChatMessagesAtLeast(t, chatStore, 2, 2*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("messages len=%d want 2; msgs=%+v", len(msgs), msgs)
	}

	for _, m := range msgs {
		if m.Role != chat.RoleAssistant {
			t.Fatalf("role=%q want assistant", m.Role)
		}
	}

	body := strings.TrimSpace(msgs[0].Content + "\n" + msgs[1].Content)
	if strings.Contains(body, "task-running") {
		t.Fatalf("unexpected running report: %q", body)
	}
	if !strings.Contains(body, "task-succeeded") {
		t.Fatalf("missing succeeded task report: %q", body)
	}
	if !strings.Contains(body, "task-blocked") {
		t.Fatalf("missing blocked task report: %q", body)
	}
}

func waitForChatMessagesAtLeast(t *testing.T, store *chat.Store, min int, timeout time.Duration) []chat.Message {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		msgs, err := store.Tail(context.Background(), 50)
		if err != nil {
			t.Fatalf("tail chat messages: %v", err)
		}
		if len(msgs) >= min {
			return msgs
		}
		if time.Now().After(deadline) {
			return msgs
		}
		time.Sleep(20 * time.Millisecond)
	}
}
