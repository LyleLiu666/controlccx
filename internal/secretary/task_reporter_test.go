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

func TestService_StartTaskStatusReporter_ForwardsSystemUserPromptAndLetsAgentReport(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	client := &scriptedClient{
		responses: []string{
			"任务 task-succeeded 已完成，我已向用户汇报。",
			"任务 task-blocked 受阻（approval required），我已向用户汇报。",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(client))

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

	msgs := waitForChatMessagesAtLeast(t, chatStore, 4, 2*time.Second)
	if len(msgs) != 4 {
		t.Fatalf("messages len=%d want 4; msgs=%+v", len(msgs), msgs)
	}

	var userMsgs []chat.Message
	var assistantMsgs []chat.Message
	for _, m := range msgs {
		switch m.Role {
		case chat.RoleUser:
			userMsgs = append(userMsgs, m)
		case chat.RoleAssistant:
			assistantMsgs = append(assistantMsgs, m)
		default:
			t.Fatalf("unexpected role=%q", m.Role)
		}
	}
	if len(userMsgs) != 2 {
		t.Fatalf("user messages=%d want 2; msgs=%+v", len(userMsgs), msgs)
	}
	if len(assistantMsgs) != 2 {
		t.Fatalf("assistant messages=%d want 2; msgs=%+v", len(assistantMsgs), msgs)
	}

	userBody := strings.TrimSpace(userMsgs[0].Content + "\n" + userMsgs[1].Content)
	if strings.Contains(userBody, "task-running") {
		t.Fatalf("unexpected running report prompt: %q", userBody)
	}
	if !strings.Contains(userBody, "【系统消息】") {
		t.Fatalf("missing system message marker in prompt: %q", userBody)
	}
	if !strings.Contains(userBody, "请你向用户汇报结果") {
		t.Fatalf("missing report instruction in prompt: %q", userBody)
	}
	if strings.Contains(userBody, "任务进展汇报：") {
		t.Fatalf("legacy fixed report template should be removed: %q", userBody)
	}
	if !strings.Contains(userBody, "task-succeeded") {
		t.Fatalf("missing succeeded task in prompt: %q", userBody)
	}
	if !strings.Contains(userBody, "task-blocked") {
		t.Fatalf("missing blocked task in prompt: %q", userBody)
	}

	assistantBody := strings.TrimSpace(assistantMsgs[0].Content + "\n" + assistantMsgs[1].Content)
	if !strings.Contains(assistantBody, "task-succeeded") {
		t.Fatalf("assistant did not report succeeded task: %q", assistantBody)
	}
	if !strings.Contains(assistantBody, "task-blocked") {
		t.Fatalf("assistant did not report blocked task: %q", assistantBody)
	}
}

func TestBuildTaskStatusSystemUserPrompt_SucceededWithWarning_IncludesWarningLine(t *testing.T) {
	out := buildTaskStatusSystemUserPrompt(tasks.Task{
		ID:         "task-1",
		Status:     tasks.StatusSucceeded,
		WorkerType: tasks.WorkerCodex,
		Prompt:     "do thing",
		Warning:    "run succeeded but tool errors were observed; see stderr logs",
	})
	if !strings.Contains(out, "【系统消息】") {
		t.Fatalf("expected system message marker, got: %q", out)
	}
	if !strings.Contains(out, "请你向用户汇报结果") {
		t.Fatalf("expected report instruction, got: %q", out)
	}
	if !strings.Contains(out, "warning:") {
		t.Fatalf("expected warning line, got: %q", out)
	}
	if !strings.Contains(out, "tool errors were observed") {
		t.Fatalf("expected warning content, got: %q", out)
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
