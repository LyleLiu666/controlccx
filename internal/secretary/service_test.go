package secretary

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/tasks"
)

type scriptedClient struct {
	responses []string
	calls     int
	messages  [][]agentsdk.Message
}

func (c *scriptedClient) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = opts
	c.messages = append(c.messages, append([]agentsdk.Message(nil), messages...))
	if c.calls >= len(c.responses) {
		return callback("no more scripted responses")
	}
	resp := c.responses[c.calls]
	c.calls++
	return callback(resp)
}

func TestService_Send_ToolLoopAndPersistsVisibleMessages(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	now := time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC)

	okTask, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "ok",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create ok task: %v", err)
	}
	exit0 := 0
	if err := taskStore.FinishTask(ctx, okTask.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		ExitCode:   &exit0,
		Error:      "",
		SessionID:  "sess-ok",
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish ok task: %v", err)
	}

	failTask, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "fail",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create fail task: %v", err)
	}
	exit1 := 1
	if err := taskStore.FinishTask(ctx, failTask.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		ExitCode:   &exit1,
		Error:      "boom",
		SessionID:  "sess-fail",
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("finish fail task: %v", err)
	}

	_, err = taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Mode:       tasks.ModeNew,
		Prompt:     "queued",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create queued task: %v", err)
	}

	c := &scriptedClient{
		responses: []string{
			"<tool_data>\n  <call>\n    <tool_name>tasks_count</tool_name>\n  </call>\n</tool_data>\n",
			"总共有 3 个任务：已完成 1 个，失败 1 个。",
		},
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	reply, err := svc.Send(ctx, "统计一下任务")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.TrimSpace(reply) != "总共有 3 个任务：已完成 1 个，失败 1 个。" {
		t.Fatalf("reply=%q", reply)
	}

	hist, err := svc.History(ctx, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len=%d want 2", len(hist))
	}
	if hist[0].Role != chat.RoleUser || strings.TrimSpace(hist[0].Content) != "统计一下任务" {
		t.Fatalf("unexpected first message: %+v", hist[0])
	}
	if hist[1].Role != chat.RoleAssistant || strings.TrimSpace(hist[1].Content) != strings.TrimSpace(reply) {
		t.Fatalf("unexpected second message: %+v", hist[1])
	}
	if strings.Contains(hist[1].Content, "<tool_result>") {
		t.Fatalf("expected visible history to exclude tool_result")
	}

	if len(c.messages) < 2 {
		t.Fatalf("expected at least 2 llm calls, got %d", len(c.messages))
	}

	var toolResultMsg string
	for _, m := range c.messages[1] {
		if m.Role == "user" && strings.Contains(m.Content, "<tool_result>") {
			toolResultMsg = m.Content
			break
		}
	}
	if strings.TrimSpace(toolResultMsg) == "" {
		t.Fatalf("expected tool_result to be present in second call messages")
	}
	outJSON := extractFirstTagValue(toolResultMsg, "output")
	if strings.TrimSpace(outJSON) == "" {
		t.Fatalf("expected <output> JSON in tool_result, got: %q", toolResultMsg)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(outJSON), &payload); err != nil {
		t.Fatalf("parse tool output json: %v (json=%q)", err, outJSON)
	}
	if payload["total"] != float64(3) {
		t.Fatalf("tool total=%v want %v", payload["total"], 3)
	}
	by, _ := payload["by_status"].(map[string]any)
	if by == nil {
		t.Fatalf("expected by_status map, got %T", payload["by_status"])
	}
	if by["succeeded"] != float64(1) || by["failed"] != float64(1) || by["queued"] != float64(1) {
		t.Fatalf("unexpected by_status=%v", by)
	}
}

func extractFirstTagValue(s string, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i+len(open):], close)
	if j < 0 {
		return ""
	}
	return s[i+len(open) : i+len(open)+j]
}

type blockingClient struct {
	mu sync.Mutex

	calls int

	firstStarted  chan struct{}
	unblockFirst  chan struct{}
	secondStarted chan struct{}
}

func (c *blockingClient) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = messages
	_ = opts

	c.mu.Lock()
	callNum := c.calls
	c.calls++
	c.mu.Unlock()

	switch callNum {
	case 0:
		close(c.firstStarted)
		<-c.unblockFirst
		return callback("reply-1")
	case 1:
		close(c.secondStarted)
		return callback("reply-2")
	default:
		return callback("no more scripted responses")
	}
}

func TestService_Send_SerializesConcurrentRequests(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	c := &blockingClient{
		firstStarted:  make(chan struct{}),
		unblockFirst:  make(chan struct{}),
		secondStarted: make(chan struct{}),
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	var (
		r1, r2 string
		e1, e2 error
	)
	firstDone := make(chan struct{})
	go func() {
		r1, e1 = svc.Send(ctx, "one")
		close(firstDone)
	}()
	<-c.firstStarted

	secondDone := make(chan struct{})
	go func() {
		r2, e2 = svc.Send(ctx, "two")
		close(secondDone)
	}()

	select {
	case <-c.secondStarted:
		t.Fatalf("expected second request to be blocked until first completes")
	case <-time.After(250 * time.Millisecond):
		// ok
	}

	close(c.unblockFirst)
	<-firstDone
	<-secondDone

	if e1 != nil || e2 != nil {
		t.Fatalf("send errors: e1=%v e2=%v", e1, e2)
	}
	if strings.TrimSpace(r1) != "reply-1" || strings.TrimSpace(r2) != "reply-2" {
		t.Fatalf("unexpected replies: r1=%q r2=%q", r1, r2)
	}

	hist, err := svc.History(ctx, 10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 4 {
		t.Fatalf("history len=%d want 4", len(hist))
	}
	if hist[0].Role != chat.RoleUser || strings.TrimSpace(hist[0].Content) != "one" {
		t.Fatalf("unexpected msg0: %+v", hist[0])
	}
	if hist[1].Role != chat.RoleAssistant || strings.TrimSpace(hist[1].Content) != "reply-1" {
		t.Fatalf("unexpected msg1: %+v", hist[1])
	}
	if hist[2].Role != chat.RoleUser || strings.TrimSpace(hist[2].Content) != "two" {
		t.Fatalf("unexpected msg2: %+v", hist[2])
	}
	if hist[3].Role != chat.RoleAssistant || strings.TrimSpace(hist[3].Content) != "reply-2" {
		t.Fatalf("unexpected msg3: %+v", hist[3])
	}
}
