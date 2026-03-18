package secretary

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/secretary/llm"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

type scriptedClient struct {
	responses []string
	calls     int
	messages  [][]agentsdk.Message
}

type receiptBackendStub struct{}

func (b *receiptBackendStub) Name() string { return "receipt-stub" }

func (b *receiptBackendStub) Complete(ctx context.Context, prompt string) (string, error) {
	_ = ctx
	_ = prompt
	return "ok", nil
}

func (b *receiptBackendStub) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	_ = ctx
	_ = messages
	_ = opts
	return "ok", nil
}

func (b *receiptBackendStub) LastReceipt() map[string]any {
	return map[string]any{
		"provider":   "test-provider",
		"request_id": "req-test-1",
		"usage": map[string]any{
			"cache_read_input_tokens": 7,
			"output_tokens":           3,
		},
		"kv_cache": map[string]any{
			"cache_read_input_tokens": 7,
		},
	}
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

func TestService_SendByConversation_IsolatesHistory(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	c := &scriptedClient{
		responses: []string{
			"A1",
			"B1",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	replyA, err := svc.SendByConversation(ctx, "conv-a", "hello a")
	if err != nil {
		t.Fatalf("send conv-a: %v", err)
	}
	if strings.TrimSpace(replyA) != "A1" {
		t.Fatalf("replyA=%q want %q", replyA, "A1")
	}

	replyB, err := svc.SendByConversation(ctx, "conv-b", "hello b")
	if err != nil {
		t.Fatalf("send conv-b: %v", err)
	}
	if strings.TrimSpace(replyB) != "B1" {
		t.Fatalf("replyB=%q want %q", replyB, "B1")
	}

	histA, err := svc.HistoryByConversation(ctx, "conv-a", 10)
	if err != nil {
		t.Fatalf("history conv-a: %v", err)
	}
	if len(histA) != 2 {
		t.Fatalf("conv-a len=%d want 2", len(histA))
	}
	if strings.TrimSpace(histA[0].Content) != "hello a" || strings.TrimSpace(histA[1].Content) != "A1" {
		t.Fatalf("unexpected conv-a history: %+v", histA)
	}

	histB, err := svc.HistoryByConversation(ctx, "conv-b", 10)
	if err != nil {
		t.Fatalf("history conv-b: %v", err)
	}
	if len(histB) != 2 {
		t.Fatalf("conv-b len=%d want 2", len(histB))
	}
	if strings.TrimSpace(histB[0].Content) != "hello b" || strings.TrimSpace(histB[1].Content) != "B1" {
		t.Fatalf("unexpected conv-b history: %+v", histB)
	}

	if len(c.messages) < 2 {
		t.Fatalf("captured messages len=%d want >=2", len(c.messages))
	}
	for _, m := range c.messages[1] {
		if strings.Contains(m.Content, "hello a") {
			t.Fatalf("conv-b llm request leaked conv-a history: %+v", c.messages[1])
		}
	}
}

func TestService_SendByConversation_DisabledConversationMemoryFallsBackToGlobal(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	c := &scriptedClient{responses: []string{"A1", "B1"}}

	cfg := config.Default()
	cfg.Secretary.ConversationMemoryEnabled = false
	svc := NewService(cfg, taskStore, chatStore, nil, nil, WithClient(c))

	if _, err := svc.SendByConversation(ctx, "conv-a", "hello a"); err != nil {
		t.Fatalf("send conv-a: %v", err)
	}
	if _, err := svc.SendByConversation(ctx, "conv-b", "hello b"); err != nil {
		t.Fatalf("send conv-b: %v", err)
	}

	histA, err := svc.HistoryByConversation(ctx, "conv-a", 20)
	if err != nil {
		t.Fatalf("history conv-a: %v", err)
	}
	histB, err := svc.HistoryByConversation(ctx, "conv-b", 20)
	if err != nil {
		t.Fatalf("history conv-b: %v", err)
	}
	if len(histA) != 4 || len(histB) != 4 {
		t.Fatalf("expected both histories to use global fallback, got conv-a=%d conv-b=%d", len(histA), len(histB))
	}
	if strings.TrimSpace(histA[0].Content) != "hello a" || strings.TrimSpace(histA[2].Content) != "hello b" {
		t.Fatalf("unexpected globalized history: %+v", histA)
	}
}

func TestService_WriteGuardStats_EmitAndBlock(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "seed",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>mission_contract_upsert</tool_name><task_id>" + task.ID + "</task_id><goal>g1</goal></call></tool_data>",
			"ok",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))
	if _, err := svc.Send(ctx, "写任务契约"); err != nil {
		t.Fatalf("send with task_id write: %v", err)
	}
	stats := svc.CurrentGuardStats()
	if stats.ActionPlanEmitted == 0 {
		t.Fatalf("expected action plan emitted > 0")
	}
	if stats.WriteGuardBlocked != 0 {
		t.Fatalf("expected no write guard block, got %d", stats.WriteGuardBlocked)
	}

	c2 := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>mission_contract_upsert</tool_name><key>c:k1</key><goal>g2</goal></call></tool_data>",
			"ok",
		},
	}
	svc2 := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c2))
	if _, err := svc2.Send(ctx, "写任务契约2"); err != nil {
		t.Fatalf("send taskless write: %v", err)
	}
	stats2 := svc2.CurrentGuardStats()
	if stats2.WriteGuardBlocked == 0 {
		t.Fatalf("expected write guard blocked > 0 for taskless write without event store")
	}
}

func TestService_WriteGuardDisabled_DoesNotBlockTasklessWrite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>mission_contract_upsert</tool_name><key>c:k2</key><goal>g2</goal></call></tool_data>",
			"ok",
		},
	}

	cfg := config.Default()
	cfg.Secretary.WriteGuardEnabled = false
	svc := NewService(cfg, taskStore, chatStore, nil, nil, WithClient(c))
	if _, err := svc.Send(ctx, "写任务契约3"); err != nil {
		t.Fatalf("send: %v", err)
	}

	stats := svc.CurrentGuardStats()
	if stats.WriteGuardBlocked != 0 {
		t.Fatalf("expected blocked=0 when guard disabled, got %d", stats.WriteGuardBlocked)
	}
	if stats.ActionPlanEmitted != 0 {
		t.Fatalf("expected emitted=0 when guard disabled, got %d", stats.ActionPlanEmitted)
	}
}

func TestService_Send_BindsMissionContractViaChatTool(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-contract-chat",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	key := tasks.SessionKeyForTask(task)

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>mission_contract_upsert</tool_name><task_id>" + task.ID + "</task_id><goal>交付可自主推进的任务闭环</goal><constraints>先对齐需求,必须可回滚</constraints><acceptance_criteria>go test ./... passes,关键路径可追踪</acceptance_criteria><non_goals>重写全部前端</non_goals></call></tool_data>",
			"我已创建任务契约草案，请你确认后我再开始执行。",
		},
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))
	reply, err := svc.Send(ctx, "先帮我写个任务契约草案")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(reply, "任务契约草案") {
		t.Fatalf("reply=%q, want mention contract draft", reply)
	}

	contract, ok, err := taskStore.GetMissionContract(ctx, key)
	if err != nil {
		t.Fatalf("get mission contract: %v", err)
	}
	if !ok {
		t.Fatalf("expected mission contract created for key %s", key)
	}
	if contract.Revision != 1 {
		t.Fatalf("revision=%d, want 1", contract.Revision)
	}
	if strings.TrimSpace(contract.Goal) == "" {
		t.Fatalf("expected non-empty goal")
	}
}

func TestService_Send_ExecutionPlanLoopViaChatToolPersistsProgress(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType:     tasks.WorkerClaudeCode,
		Mode:           tasks.ModeNew,
		ConversationID: "conv-loop-chat",
		Prompt:         "seed",
		WorkDir:        ".",
		SessionID:      "sess-loop-chat",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusFailed,
		Error:      "boom",
		SessionID:  task.SessionID,
		FinishedAt: task.CreatedAt.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	contractKey := tasks.ConversationKey(task.ConversationID)
	if _, err := taskStore.UpsertMissionContract(ctx, tasks.UpsertMissionContractInput{
		Key:  contractKey,
		Goal: "deliver autonomous loop",
	}); err != nil {
		t.Fatalf("upsert mission contract: %v", err)
	}
	if _, err := taskStore.ConfirmMissionContract(ctx, contractKey); err != nil {
		t.Fatalf("confirm mission contract: %v", err)
	}

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>execution_plan_loop_submit</tool_name><task_id>" + task.ID + "</task_id><max_iterations>1</max_iterations></call></tool_data>",
			"我已按计划推进第 1 步，并记录进度。",
		},
	}

	ops := &taskops.Service{Tasks: taskStore}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c), WithTaskOps(ops))
	reply, err := svc.Send(ctx, "按契约开始自动推进")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(reply, "第 1 步") {
		t.Fatalf("reply=%q, want mention step progress", reply)
	}

	state, ok, err := taskStore.GetExecutionPlanState(ctx, contractKey)
	if err != nil {
		t.Fatalf("get execution plan state: %v", err)
	}
	if !ok {
		t.Fatalf("expected execution plan state")
	}
	if state.Iteration != 1 {
		t.Fatalf("state.iteration=%d, want 1", state.Iteration)
	}
}

func TestService_Send_GeneratesRollbackPlaybookViaChatTool(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerClaudeCode,
		Mode:       tasks.ModeNew,
		Prompt:     "seed",
		WorkDir:    ".",
		SessionID:  "sess-playbook-chat",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := taskStore.CreateRollbackProof(ctx, tasks.CreateRollbackProofInput{
		TaskID:     task.ID,
		ActionType: "git.remote",
		ActionRef:  "push-main",
		ProofType:  "snapshot",
		ProofRef:   "snapshot://rev-1",
		Detail:     json.RawMessage(`{"workspace":"rev-1"}`),
	}); err != nil {
		t.Fatalf("create rollback proof: %v", err)
	}

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>rollback_playbook_generate</tool_name><task_id>" + task.ID + "</task_id></call></tool_data>",
			"回滚步骤如下：先使用 snapshot://rev-1 恢复，再验证关键测试。",
		},
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))
	reply, err := svc.Send(ctx, "给我回滚方案")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(reply, "snapshot://rev-1") {
		t.Fatalf("reply=%q, want proof reference", reply)
	}
}

func TestService_Send_PersistsEventSinkEvents(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	eventStore := NewEventStore(conn)

	c := &scriptedClient{
		responses: []string{"hello"},
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c), WithEventStore(eventStore))

	reply, err := svc.Send(ctx, "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.TrimSpace(reply) != "hello" {
		t.Fatalf("reply=%q", reply)
	}

	events, err := eventStore.Tail(ctx, 500)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("events len=%d want >=2", len(events))
	}

	runID := strings.TrimSpace(events[0].RunID)
	if runID == "" {
		t.Fatalf("expected non-empty run_id")
	}
	hasRequest := false
	for _, ev := range events {
		if strings.TrimSpace(ev.RunID) != runID {
			t.Fatalf("mixed run_id: %q vs %q", ev.RunID, runID)
		}
		if ev.Kind == agentsdk.EventKindLLMRequest {
			hasRequest = true
		}
		if strings.TrimSpace(ev.EventJSON) == "" {
			t.Fatalf("empty event_json for kind=%q", ev.Kind)
		}
		var anyPayload any
		if err := json.Unmarshal([]byte(ev.EventJSON), &anyPayload); err != nil {
			t.Fatalf("unmarshal event_json: %v", err)
		}
	}
	if !hasRequest {
		t.Fatalf("expected at least one llm_request event")
	}
}

func TestService_Send_PersistsProviderReceiptEvent_WhenBackendSupportsIt(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	eventStore := NewEventStore(conn)

	client := &llm.Client{Backend: &receiptBackendStub{}}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(client), WithEventStore(eventStore))

	reply, err := svc.Send(ctx, "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.TrimSpace(reply) != "ok" {
		t.Fatalf("reply=%q want %q", reply, "ok")
	}

	events, err := eventStore.Tail(ctx, 500)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	found := false
	for _, ev := range events {
		if string(ev.Kind) != "provider_receipt" {
			continue
		}
		if !strings.Contains(ev.EventJSON, "req-test-1") {
			t.Fatalf("provider_receipt missing request id, event=%s", ev.EventJSON)
		}
		found = true
	}
	if !found {
		t.Fatalf("expected provider_receipt event in secretary_events")
	}
}

func TestService_Send_PersistsTranscriptAndUsesItInNextLLMRequest(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	task, err := taskStore.CreateTask(ctx, tasks.CreateTaskInput{
		WorkerType: tasks.WorkerExec,
		Prompt:     "seed",
		WorkDir:    ".",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := taskStore.FinishTask(ctx, task.ID, tasks.FinishTaskInput{
		Status:     tasks.StatusSucceeded,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>tasks_count</tool_name><status>succeeded</status></call></tool_data>",
			"目前已完成（succeeded）的任务共有 1 个。",
			"ok",
		},
	}

	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	reply1, err := svc.Send(ctx, "已完成（succeeded）的有多少？")
	if err != nil {
		t.Fatalf("send 1: %v", err)
	}
	if !strings.Contains(reply1, "1") {
		t.Fatalf("reply1=%q want count", reply1)
	}

	_, err = svc.Send(ctx, "有一个任务是重构前端代码的 你看看是否可以继续")
	if err != nil {
		t.Fatalf("send 2: %v", err)
	}

	if len(c.messages) < 3 {
		t.Fatalf("captured llm requests=%d want >=3", len(c.messages))
	}
	req := c.messages[2]

	var (
		firstUserIdx      = -1
		toolAssistantIdx  = -1
		toolResultUserIdx = -1
		firstAnswerIdx    = -1
		secondUserIdx     = -1
	)

	for i, m := range req {
		if strings.Contains(m.Content, "已完成（succeeded）的有多少？") {
			firstUserIdx = i
		}
		if m.Role == "assistant" && strings.Contains(m.Content, "<tool_data>") && strings.Contains(m.Content, "tasks_count") {
			toolAssistantIdx = i
		}
		if m.Role == "user" && strings.Contains(m.Content, "<tool_result>") && strings.Contains(m.Content, "tasks_count") {
			toolResultUserIdx = i
		}
		if m.Role == "assistant" && strings.Contains(m.Content, "目前已完成") {
			firstAnswerIdx = i
		}
		if m.Role == "user" && strings.Contains(m.Content, "有一个任务是重构前端代码的 你看看是否可以继续") {
			secondUserIdx = i
		}
	}
	if firstUserIdx == -1 || toolAssistantIdx == -1 || toolResultUserIdx == -1 || firstAnswerIdx == -1 || secondUserIdx == -1 {
		t.Fatalf("expected full transcript in next request, got: %#v", req)
	}
	if !(firstUserIdx < toolAssistantIdx &&
		toolAssistantIdx < toolResultUserIdx &&
		toolResultUserIdx < firstAnswerIdx &&
		firstAnswerIdx < secondUserIdx) {
		t.Fatalf(
			"unexpected transcript order: firstUser=%d toolAssistant=%d toolResult=%d firstAnswer=%d secondUser=%d",
			firstUserIdx, toolAssistantIdx, toolResultUserIdx, firstAnswerIdx, secondUserIdx,
		)
	}

	rawHistory, err := chatStore.Tail(ctx, 20)
	if err != nil {
		t.Fatalf("tail raw history: %v", err)
	}
	hasToolData := false
	hasToolResult := false
	for _, m := range rawHistory {
		text := strings.TrimSpace(m.Content)
		if m.Role == chat.RoleAssistant && strings.Contains(text, "<tool_data>") {
			hasToolData = true
		}
		if m.Role == chat.RoleUser && strings.Contains(text, "<tool_result>") {
			hasToolResult = true
		}
	}
	if !hasToolData || !hasToolResult {
		t.Fatalf("expected raw chat history to include tool_data/tool_result messages")
	}

	visibleHistory, err := svc.History(ctx, 20)
	if err != nil {
		t.Fatalf("visible history: %v", err)
	}
	for _, m := range visibleHistory {
		if strings.Contains(m.Content, "<tool_data>") || strings.Contains(m.Content, "<tool_result>") {
			t.Fatalf("expected visible history to hide internal transcript messages, got: %+v", m)
		}
	}
}

func TestService_SendStream_EmitsVisibleAndToolHooks(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	c := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>tasks_count</tool_name></call></tool_data>",
			"ok",
		},
	}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	var (
		deltas      []string
		toolCalls   []string
		toolResults []string
		traceLines  []string
	)
	reply, err := svc.SendStream(ctx, "hi", &SendHooks{
		OnVisibleDelta: func(delta string) error {
			deltas = append(deltas, delta)
			return nil
		},
		OnTrace: func(step int, message string) {
			traceLines = append(traceLines, strings.TrimSpace(message))
		},
		OnToolCall: func(step int, event agentsdk.ToolCallEvent) {
			toolCalls = append(toolCalls, strings.TrimSpace(event.Name))
		},
		OnToolResult: func(step int, event agentsdk.ToolResultEvent) {
			toolResults = append(toolResults, strings.TrimSpace(event.ToolName))
		},
	})
	if err != nil {
		t.Fatalf("send stream: %v", err)
	}
	if strings.TrimSpace(reply) != "ok" {
		t.Fatalf("reply=%q want %q", reply, "ok")
	}
	if got := strings.TrimSpace(strings.Join(deltas, "")); got != "ok" {
		t.Fatalf("visible deltas=%q want %q", got, "ok")
	}
	if len(toolCalls) == 0 || toolCalls[0] != "tasks_count" {
		t.Fatalf("expected tool call hook for tasks_count, got: %#v", toolCalls)
	}
	if len(toolResults) == 0 || toolResults[0] != "tasks_count" {
		t.Fatalf("expected tool result hook for tasks_count, got: %#v", toolResults)
	}
	if len(traceLines) == 0 {
		t.Fatalf("expected at least one trace line")
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

func TestService_History_PaginatesVisibleMessagesWhenInternalTranscriptIsDense(t *testing.T) {
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

	// Seed visible messages first.
	for i := 0; i < 260; i++ {
		if _, err := chatStore.Append(ctx, chat.RoleUser, "visible-"+pad3(i)); err != nil {
			t.Fatalf("append visible %d: %v", i, err)
		}
	}
	// Then append many internal transcript pairs at the tail.
	for i := 0; i < 160; i++ {
		if _, err := chatStore.Append(ctx, chat.RoleAssistant, "<tool_data><call><tool_name>tasks_count</tool_name><i>"+pad3(i)+"</i></call></tool_data>"); err != nil {
			t.Fatalf("append tool_data %d: %v", i, err)
		}
		if _, err := chatStore.Append(ctx, chat.RoleUser, "<tool_result><call><tool_name>tasks_count</tool_name><i>"+pad3(i)+"</i></call></tool_result>"); err != nil {
			t.Fatalf("append tool_result %d: %v", i, err)
		}
	}

	hist, err := svc.History(ctx, 200)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 200 {
		t.Fatalf("history len=%d want 200", len(hist))
	}
	if got := strings.TrimSpace(hist[0].Content); got != "visible-060" {
		t.Fatalf("first visible=%q want %q", got, "visible-060")
	}
	if got := strings.TrimSpace(hist[len(hist)-1].Content); got != "visible-259" {
		t.Fatalf("last visible=%q want %q", got, "visible-259")
	}
	for _, m := range hist {
		if strings.Contains(m.Content, "<tool_data>") || strings.Contains(m.Content, "<tool_result>") {
			t.Fatalf("expected visible history to hide internal transcript messages, got: %+v", m)
		}
	}
}

func TestService_History_FiltersInternalTranscriptByRole(t *testing.T) {
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

	cases := []struct {
		role    chat.Role
		content string
	}{
		{role: chat.RoleUser, content: "normal-user"},
		{role: chat.RoleAssistant, content: "normal-assistant"},
		{role: chat.RoleUser, content: "用户示例：<tool_data>这不是内部协议</tool_data>"},
		{role: chat.RoleAssistant, content: "助手示例：<tool_result>这不是内部协议</tool_result>"},
		{role: chat.RoleAssistant, content: "<tool_data><call><tool_name>tasks_count</tool_name></call></tool_data>"},
		{role: chat.RoleUser, content: "<tool_result><call><tool_name>tasks_count</tool_name></call></tool_result>"},
		{role: chat.RoleAssistant, content: "tail-visible"},
	}
	for i, tc := range cases {
		if _, err := chatStore.Append(ctx, tc.role, tc.content); err != nil {
			t.Fatalf("append case %d: %v", i, err)
		}
	}

	hist, err := svc.History(ctx, 20)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 5 {
		t.Fatalf("history len=%d want 5", len(hist))
	}

	want := []string{
		"normal-user",
		"normal-assistant",
		"用户示例：<tool_data>这不是内部协议</tool_data>",
		"助手示例：<tool_result>这不是内部协议</tool_result>",
		"tail-visible",
	}
	for i := range want {
		if strings.TrimSpace(hist[i].Content) != want[i] {
			t.Fatalf("hist[%d]=%q want %q", i, hist[i].Content, want[i])
		}
	}
}

func TestService_Send_AutoCompressesHistory(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	compressStore := NewCompressionStore(conn)

	// Seed a long-enough history to force compression with a small max context.
	for i := 0; i < 6; i++ { // 12 messages
		if _, err := chatStore.Append(ctx, chat.RoleUser, strings.Repeat("u", 600)); err != nil {
			t.Fatalf("append user: %v", err)
		}
		if _, err := chatStore.Append(ctx, chat.RoleAssistant, strings.Repeat("a", 600)); err != nil {
			t.Fatalf("append assistant: %v", err)
		}
	}

	c := &scriptedClient{
		responses: []string{
			"这是压缩后的摘要",
			"final",
		},
	}

	compOpts := DefaultCompressionOptions()
	compOpts.MaxContextRunes = 2000
	compOpts.KeepTextMessages = 4
	compOpts.MaxCompressionSteps = 1

	svc := NewService(
		config.Default(),
		taskStore,
		chatStore,
		nil,
		nil,
		WithClient(c),
		WithCompressionStore(compressStore),
		WithCompressionOptions(compOpts),
	)

	reply, err := svc.Send(ctx, "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.TrimSpace(reply) != "final" {
		t.Fatalf("reply=%q", reply)
	}

	rec, ok, err := compressStore.Latest(ctx)
	if err != nil {
		t.Fatalf("latest compression: %v", err)
	}
	if !ok {
		t.Fatalf("expected compression record")
	}
	if strings.TrimSpace(rec.Summary) != "这是压缩后的摘要" {
		t.Fatalf("summary=%q", rec.Summary)
	}
	if rec.CursorAfter <= 0 {
		t.Fatalf("expected cursor_after to advance, got %d", rec.CursorAfter)
	}

	if len(c.messages) != 2 {
		t.Fatalf("expected 2 llm calls (compress + answer), got %d", len(c.messages))
	}
	main := c.messages[1]
	if len(main) > 10 {
		t.Fatalf("expected compressed prompt history, got %d messages", len(main))
	}

	hasSummary := false
	for _, m := range main {
		if m.Role == "system" && strings.Contains(m.Content, "对话摘要") {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		t.Fatalf("expected summary to be injected into main prompt")
	}
}

func pad3(i int) string {
	if i < 10 {
		return "00" + strconv.Itoa(i)
	}
	if i < 100 {
		return "0" + strconv.Itoa(i)
	}
	return strconv.Itoa(i)
}

func TestSecretaryFailedMessage_DoesNotMentionCLIBackends(t *testing.T) {
	msg := secretaryFailedMessage("simple-http", errors.New("boom"))
	if strings.Contains(strings.ToLower(msg), "paths.claude") || strings.Contains(strings.ToLower(msg), "paths.codex") {
		t.Fatalf("expected failure message to not mention CLI paths, got: %q", msg)
	}
	if strings.Contains(strings.ToLower(msg), "claude/codex") {
		t.Fatalf("expected failure message to not mention cli backends, got: %q", msg)
	}
}

func TestService_Send_AllowsLongerToolLoopsBeyondLegacy60Steps(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "controlccx.db")

	conn, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	const toolSteps = 65
	responses := make([]string, 0, toolSteps+1)
	toolCall := "<tool_data><call><tool_name>system_info</tool_name></call></tool_data>"
	for i := 0; i < toolSteps; i++ {
		responses = append(responses, toolCall)
	}
	responses = append(responses, "done")

	c := &scriptedClient{responses: responses}
	svc := NewService(config.Default(), taskStore, chatStore, nil, nil, WithClient(c))

	reply, err := svc.Send(ctx, "loop")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if strings.TrimSpace(reply) != "done" {
		t.Fatalf("expected final reply %q, got %q", "done", reply)
	}
	if c.calls != toolSteps+1 {
		t.Fatalf("llm calls=%d want %d", c.calls, toolSteps+1)
	}
}
