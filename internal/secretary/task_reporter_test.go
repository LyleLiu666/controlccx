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
			"任务 task-awaiting 需要审批，我已处理。",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(client))

	hub := events.NewHub()
	stop := svc.StartTaskStatusReporter(context.Background(), hub)
	defer stop()

	awaitingTask, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "run tests",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create awaiting task: %v", err)
	}

	approval, err := taskStore.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     awaitingTask.ID,
		WorkerType: tasks.WorkerCodex,
		WorkDir:    ".",
		ActionType: "execCommandApproval",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "go test ./...",
		Raw:        []byte(`{"cmd":"go test ./..."}`),
	})
	if err != nil {
		t.Fatalf("create approval request: %v", err)
	}

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
	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         awaitingTask.ID,
			Status:     tasks.StatusAwaitingApproval,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "run tests",
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

	msgs := waitForChatMessagesAtLeast(t, chatStore, 6, 2*time.Second)
	if len(msgs) != 6 {
		t.Fatalf("messages len=%d want 6; msgs=%+v", len(msgs), msgs)
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
	if len(userMsgs) != 3 {
		t.Fatalf("user messages=%d want 3; msgs=%+v", len(userMsgs), msgs)
	}
	if len(assistantMsgs) != 3 {
		t.Fatalf("assistant messages=%d want 3; msgs=%+v", len(assistantMsgs), msgs)
	}

	userBody := strings.TrimSpace(userMsgs[0].Content + "\n" + userMsgs[1].Content + "\n" + userMsgs[2].Content)
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

	assistantBody := strings.TrimSpace(assistantMsgs[0].Content + "\n" + assistantMsgs[1].Content + "\n" + assistantMsgs[2].Content)
	if !strings.Contains(assistantBody, "task-succeeded") {
		t.Fatalf("assistant did not report succeeded task: %q", assistantBody)
	}
	if !strings.Contains(assistantBody, "task-blocked") {
		t.Fatalf("assistant did not report blocked task: %q", assistantBody)
	}

	var approvalPrompt string
	for _, m := range userMsgs {
		if strings.Contains(m.Content, "task_approval_decide") {
			approvalPrompt = m.Content
			break
		}
	}
	if approvalPrompt == "" {
		t.Fatalf("missing awaiting approval prompt; msgs=%+v", userMsgs)
	}
	if strings.Contains(approvalPrompt, "不要调用工具") {
		t.Fatalf("approval prompt should allow tool usage: %q", approvalPrompt)
	}
	if !strings.Contains(approvalPrompt, awaitingTask.ID) {
		t.Fatalf("approval prompt missing task id: %q", approvalPrompt)
	}
	if !strings.Contains(approvalPrompt, approval.ID) {
		t.Fatalf("approval prompt missing approval id: %q", approvalPrompt)
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

func TestService_StartTaskStatusReporter_ForwardsAwaitingApprovalOncePerTransition(t *testing.T) {
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
			"已收到第一次审批请求。",
			"已收到第二次审批请求。",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(client))

	hub := events.NewHub()
	stop := svc.StartTaskStatusReporter(context.Background(), hub)
	defer stop()

	approvalTask, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerCodex,
		Mode:       tasks.ModeNew,
		Prompt:     "approval",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create approval task: %v", err)
	}

	first, err := taskStore.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     approvalTask.ID,
		WorkerType: tasks.WorkerCodex,
		WorkDir:    ".",
		ActionType: "execCommandApproval",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "first approval",
		Raw:        []byte(`{"cmd":"echo first"}`),
	})
	if err != nil {
		t.Fatalf("create first approval request: %v", err)
	}

	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         approvalTask.ID,
			Status:     tasks.StatusAwaitingApproval,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "first",
		},
	})
	_ = waitForChatMessagesAtLeast(t, chatStore, 2, 2*time.Second)

	if err := taskStore.UpdateApprovalRequestDecision(ctx, first.ID, tasks.UpdateApprovalRequestDecisionInput{
		Status: tasks.ApprovalStatusApproved,
		Reason: "approved in test",
	}); err != nil {
		t.Fatalf("update first approval decision: %v", err)
	}

	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         approvalTask.ID,
			Status:     tasks.StatusRunning,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "running",
		},
	})

	second, err := taskStore.CreateApprovalRequest(ctx, tasks.CreateApprovalRequestInput{
		TaskID:     approvalTask.ID,
		WorkerType: tasks.WorkerCodex,
		WorkDir:    ".",
		ActionType: "execCommandApproval",
		RiskLevel:  tasks.RiskMedium,
		Summary:    "second approval",
		Raw:        []byte(`{"cmd":"echo second"}`),
	})
	if err != nil {
		t.Fatalf("create second approval request: %v", err)
	}

	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         approvalTask.ID,
			Status:     tasks.StatusAwaitingApproval,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "second",
		},
	})

	msgs := waitForChatMessagesAtLeast(t, chatStore, 4, 2*time.Second)
	if len(msgs) != 4 {
		t.Fatalf("messages len=%d want 4; msgs=%+v", len(msgs), msgs)
	}

	var approvalPrompts []string
	for _, m := range msgs {
		if m.Role != chat.RoleUser {
			continue
		}
		if strings.Contains(m.Content, "task_approval_decide") {
			approvalPrompts = append(approvalPrompts, m.Content)
		}
	}
	if len(approvalPrompts) != 2 {
		t.Fatalf("approval prompts=%d want 2; prompts=%q", len(approvalPrompts), approvalPrompts)
	}

	allPrompts := strings.Join(approvalPrompts, "\n")
	if !strings.Contains(allPrompts, first.ID) {
		t.Fatalf("missing first approval id in prompts: %q", allPrompts)
	}
	if !strings.Contains(allPrompts, second.ID) {
		t.Fatalf("missing second approval id in prompts: %q", allPrompts)
	}
}

func TestService_StartTaskStatusReporter_ProactiveModeOff_NoForward(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	client := &scriptedClient{responses: []string{"should-not-send"}}

	cfg := config.Default()
	cfg.Secretary.ProactiveEnabled = "off"
	svc := NewService(cfg, taskStore, chatStore, nil, nil, WithClient(client))

	hub := events.NewHub()
	stop := svc.StartTaskStatusReporter(context.Background(), hub)
	defer stop()

	hub.Publish(events.Event{
		Type: "task.updated",
		Time: time.Now().UTC(),
		Payload: tasks.Task{
			ID:         "task-succeeded",
			Status:     tasks.StatusSucceeded,
			WorkerType: tasks.WorkerCodex,
			Prompt:     "do work",
		},
	})

	time.Sleep(200 * time.Millisecond)
	msgs, err := chatStore.Tail(ctx, 20)
	if err != nil {
		t.Fatalf("tail chat messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected no forwarded messages when proactive mode off, got %d", len(msgs))
	}
}

func TestService_StartTaskStatusReporter_Aggressive_ReportsRunning(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	client := &scriptedClient{responses: []string{"已收到运行中汇报。"}}

	cfg := config.Default()
	cfg.Secretary.ProactiveEnabled = "aggressive"
	svc := NewService(cfg, taskStore, chatStore, nil, nil, WithClient(client))

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

	msgs := waitForChatMessagesAtLeast(t, chatStore, 2, 2*time.Second)
	if len(msgs) != 2 {
		t.Fatalf("messages len=%d want 2; msgs=%+v", len(msgs), msgs)
	}
	if msgs[0].Role != chat.RoleUser {
		t.Fatalf("first role=%q want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "task-running") {
		t.Fatalf("running prompt missing task id: %q", msgs[0].Content)
	}
	if msgs[1].Role != chat.RoleAssistant {
		t.Fatalf("second role=%q want assistant", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "运行中汇报") {
		t.Fatalf("assistant reply=%q", msgs[1].Content)
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
