package observer

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"controlccx/internal/chat"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

type promptProbeBackend struct {
	wantSubstr string
}

func (b *promptProbeBackend) Name() string { return "prompt-probe" }

func (b *promptProbeBackend) Complete(ctx context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, b.wantSubstr) {
		return `{"action":"final","message":"OK"}`, nil
	}
	return `{"action":"final","message":"NO_CONTEXT"}`, nil
}

func TestObserver_AgentIncludesChatHistory(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	oldest := "早先内容：你刚才问的是哪个 session；我说的是这个。"
	backend := &promptProbeBackend{wantSubstr: "哪个 session"}

	obs := &Service{
		Store:      taskStore,
		Chat:       chatStore,
		LLM:        backend,
		ForceAgent: true,
	}

	// Older assistant message that would be dropped if we only send a small tail.
	if _, err := chatStore.Append(ctx, chat.RoleAssistant, oldest); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	// Fill with enough messages so the oldest one falls outside any small tail window.
	for i := 0; i < 20; i++ {
		role := chat.RoleUser
		if i%2 == 1 {
			role = chat.RoleAssistant
		}
		if _, err := chatStore.Append(ctx, role, "filler "+strconv.Itoa(i)); err != nil {
			t.Fatalf("append filler %d: %v", i, err)
		}
	}

	// API appends the current user message before calling observer.
	current := "sess-123"
	if _, err := chatStore.Append(ctx, chat.RoleUser, current); err != nil {
		t.Fatalf("append user: %v", err)
	}

	reply, err := obs.RespondWithOptions(ctx, current, RespondOptions{Backend: "auto"})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if reply.Message != "OK" {
		t.Fatalf("reply=%q, want OK (agent should see recent chat context)", reply.Message)
	}
}

func TestObserver_AgentIncludesProjectContext(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)

	if _, err := taskStore.SetProjectContext(ctx, "Project: CCX"); err != nil {
		t.Fatalf("SetProjectContext: %v", err)
	}

	backend := &promptProbeBackend{wantSubstr: "Project: CCX"}
	obs := &Service{Store: taskStore, LLM: backend}

	reply, err := obs.Respond(ctx, "hello")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if reply.Message != "OK" {
		t.Fatalf("reply=%q, want OK (agent should see project context)", reply.Message)
	}
}

func TestObserver_ChatHistoryTool_DefaultsToMostRecent(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	obs := &Service{Store: taskStore, Chat: chatStore}

	// Create enough messages so "recent" differs from "first page".
	for i := 1; i <= 60; i++ {
		role := chat.RoleUser
		if i%2 == 0 {
			role = chat.RoleAssistant
		}
		if _, err := chatStore.Append(ctx, role, "m"+strconv.Itoa(i)); err != nil {
			t.Fatalf("append msg %d: %v", i, err)
		}
	}

	tools := obs.agentTools()
	out, err := tools["chat_history"].Run(ctx, map[string]any{"limit": 10})
	if err != nil {
		t.Fatalf("tool run: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("tool output type=%T, want map", out)
	}
	msgs, ok := m["messages"].([]chat.Message)
	if !ok {
		t.Fatalf("tool messages type=%T, want []chat.Message", m["messages"])
	}
	if len(msgs) != 10 {
		t.Fatalf("len(messages)=%d, want 10", len(msgs))
	}
	if msgs[0].Content != "m51" || msgs[len(msgs)-1].Content != "m60" {
		t.Fatalf("messages content range=%q..%q, want m51..m60", msgs[0].Content, msgs[len(msgs)-1].Content)
	}
}
