package secretary

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
	"controlccx/internal/db"
	sectools "controlccx/internal/secretary/tools"
	"controlccx/internal/tasks"
)

func TestTools_TasksCount_RejectsUnknownStatus(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	reg := newToolRegistry(taskStore)

	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "tasks_count",
		Fields: map[string]string{"status": "not-a-status"},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTools_TasksList_TruncatesUTF8Safely(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	long := strings.Repeat("中", 300)
	if _, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     long,
		WorkDir:    ".",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	reg := newToolRegistry(taskStore)
	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name:   "tasks_list",
		Fields: map[string]string{"limit": "1", "include_deleted": "1"},
	})
	if err != nil {
		t.Fatalf("execute tasks_list: %v", err)
	}
	out, ok := outAny.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type %T", outAny)
	}
	var list []struct {
		Prompt string `json:"prompt"`
	}
	raw, err := json.Marshal(out["tasks"])
	if err != nil {
		t.Fatalf("marshal tasks payload: %v", err)
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal tasks payload: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("unexpected tasks payload len=%d raw=%s", len(list), string(raw))
	}

	got := list[0].Prompt
	want := strings.Repeat("中", 239) + "…"
	if got != want {
		t.Fatalf("prompt truncated mismatch: got=%q want=%q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid utf-8, got: %q", got)
	}
}

func TestTools_TaskNewSubmit_DescriptionClarifiesWorkerTypeSemantics(t *testing.T) {
	descs := sectools.Descriptors()
	var found bool
	for _, d := range descs {
		if strings.TrimSpace(d.Name) != "task_new_submit" {
			continue
		}
		found = true
		desc := strings.TrimSpace(d.DescriptionZH)
		wants := []string{
			"worker_type 仅允许 claude-code | codex | exec",
			"claude-code=Claude Code 代理执行",
			"codex=Codex 代理执行",
			"exec=在本机 workdir 直接执行你提供的 shell（bash）命令",
			"不会做自然语言转译",
			"prompt 必须是可直接执行的命令字符串",
			"由 worker 进程执行，不是秘书自身执行",
		}
		for _, want := range wants {
			if !strings.Contains(desc, want) {
				t.Fatalf("task_new_submit description missing %q: %s", want, desc)
			}
		}
		break
	}
	if !found {
		t.Fatalf("task_new_submit descriptor not found")
	}
}
