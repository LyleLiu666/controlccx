package observer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

func TestObserver_LengthQuery_FromPromptRequirement(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	obs := &Service{Store: store}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一个10字脱口秀",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, strings.Repeat("哈", 10))

	reply, err := obs.Respond(ctx, "刚刚那个写脱口秀的任务 结果够不够字数")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, task.ID[:8]) {
		t.Fatalf("expected task id prefix in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, ">= 10") {
		t.Fatalf("expected requirement in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, "够") {
		t.Fatalf("expected '够' conclusion in reply: %q", reply.Message)
	}
}

func TestObserver_LengthQuery_NoRequirement(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := tasks.NewStore(conn)
	obs := &Service{Store: store}

	task, err := store.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "写一段脱口秀",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, _ = store.AppendLog(ctx, task.ID, tasks.LogAssistant, "你好世界")

	reply, err := obs.Respond(ctx, "刚刚那个写脱口秀的任务 结果够不够字数")
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if !strings.Contains(reply.Message, task.ID[:8]) {
		t.Fatalf("expected task id prefix in reply: %q", reply.Message)
	}
	if !strings.Contains(reply.Message, "至少") {
		t.Fatalf("expected asking for threshold in reply: %q", reply.Message)
	}
}

