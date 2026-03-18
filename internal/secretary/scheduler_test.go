package secretary

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/events"
	sectools "controlccx/internal/secretary/tools"
	"controlccx/internal/tasks"
)

type schedulerScriptClient struct {
	mu sync.Mutex

	responses    []string
	defaultReply string
	scheduleID   string

	calls    int
	messages [][]agentsdk.Message
}

func (c *schedulerScriptClient) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = opts

	c.mu.Lock()
	copied := append([]agentsdk.Message(nil), messages...)
	c.messages = append(c.messages, copied)
	idx := c.calls
	c.calls++
	out := c.defaultReply
	if idx < len(c.responses) {
		out = c.responses[idx]
	}
	out = strings.ReplaceAll(out, "{{SCHEDULE_ID}}", c.scheduleID)
	c.mu.Unlock()

	return callback(out)
}

func (c *schedulerScriptClient) setScheduleID(id string) {
	c.mu.Lock()
	c.scheduleID = strings.TrimSpace(id)
	c.mu.Unlock()
}

func (c *schedulerScriptClient) sawTimerToolResult(scheduleID string) bool {
	needle := `"source":"timer"`
	idNeedle := `"schedule_id":"` + scheduleID + `"`

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, req := range c.messages {
		for _, m := range req {
			if m.Role != "user" {
				continue
			}
			if !strings.Contains(m.Content, "<tool_result>") {
				continue
			}
			if strings.Contains(m.Content, needle) && strings.Contains(m.Content, idNeedle) {
				return true
			}
		}
	}
	return false
}

type schedulerBlockingClient struct {
	mu sync.Mutex

	callbackStarted chan struct{}
	unblockCallback chan struct{}
	sendStarted     chan struct{}

	callbackStartOnce sync.Once
	sendStartOnce     sync.Once
	unblockOnce       sync.Once
}

func newSchedulerBlockingClient() *schedulerBlockingClient {
	return &schedulerBlockingClient{
		callbackStarted: make(chan struct{}),
		unblockCallback: make(chan struct{}),
		sendStarted:     make(chan struct{}),
	}
}

func (c *schedulerBlockingClient) unblock() {
	if c == nil {
		return
	}
	c.unblockOnce.Do(func() {
		close(c.unblockCallback)
	})
}

func (c *schedulerBlockingClient) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = opts

	isTimerCallback := false
	isUserSend := false
	for _, m := range messages {
		if m.Role != "user" {
			continue
		}
		if strings.Contains(m.Content, "<tool_result>") && strings.Contains(m.Content, `"source":"timer"`) {
			isTimerCallback = true
		}
		if strings.TrimSpace(m.Content) == "hello" {
			isUserSend = true
		}
	}

	if isTimerCallback {
		c.callbackStartOnce.Do(func() { close(c.callbackStarted) })
		<-c.unblockCallback
		return callback("回调结束")
	}
	if isUserSend {
		c.sendStartOnce.Do(func() { close(c.sendStarted) })
		return callback("send-ok")
	}
	return callback("ok")
}

func setupSchedulerServiceTest(t *testing.T, client agentsdk.Client) (context.Context, *Service, *chat.Store, *EventStore, *events.Hub) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)
	eventStore := NewEventStore(conn)
	hub := events.NewHub()

	svc := NewService(
		config.Default(),
		taskStore,
		chatStore,
		nil,
		nil,
		WithClient(client),
		WithEventStore(eventStore),
		WithEventHub(hub),
	)
	return ctx, svc, chatStore, eventStore, hub
}

func waitForCondition(timeout time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func waitForHubEventType(ch <-chan events.Event, eventType string, timeout time.Duration) (events.Event, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return events.Event{}, false
		case evt, ok := <-ch:
			if !ok {
				return events.Event{}, false
			}
			if evt.Type == eventType {
				return evt, true
			}
		}
	}
}

func scheduleExists(svc *Service, scheduleID string) bool {
	if svc == nil {
		return false
	}
	svc.scheduleMu.Lock()
	defer svc.scheduleMu.Unlock()
	_, ok := svc.schedules[strings.TrimSpace(scheduleID)]
	return ok
}

func TestService_SchedulerCreate_EnforcesReadOnlyByDefault(t *testing.T) {
	ctx, svc, _, _, _ := setupSchedulerServiceTest(t, &schedulerScriptClient{defaultReply: "ok"})

	_, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "task_new_submit",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         60,
		AllowWrite:     false,
	})
	if err == nil {
		t.Fatalf("expected read-only policy error")
	}

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "task_new_submit",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         60,
		AllowWrite:     true,
	})
	if err != nil {
		t.Fatalf("allow_write=true should pass create: %v", err)
	}
	_, _ = svc.CancelSchedule(ctx, info.ID)
}

func TestService_SchedulerTick_ProducesCallbackAuditAndSSE(t *testing.T) {
	client := &schedulerScriptClient{defaultReply: "定时回调汇报"}
	ctx, svc, chatStore, eventStore, hub := setupSchedulerServiceTest(t, client)

	ch, unsubscribe := hub.Subscribe(256)
	defer unsubscribe()

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         3,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	defer func() { _, _ = svc.CancelSchedule(context.Background(), info.ID) }()

	ok := waitForCondition(4*time.Second, func() bool {
		msgs, err := chatStore.Tail(context.Background(), 20)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.Role == chat.RoleAssistant && strings.Contains(m.Content, "定时回调汇报") {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Fatalf("expected callback assistant message")
	}

	if !client.sawTimerToolResult(info.ID) {
		t.Fatalf("expected callback llm request to contain timer tool_result markers")
	}

	if _, ok := waitForHubEventType(ch, "secretary.thinking", 2*time.Second); !ok {
		t.Fatalf("expected secretary.thinking event")
	}
	if _, ok := waitForHubEventType(ch, "secretary.message", 2*time.Second); !ok {
		t.Fatalf("expected secretary.message event")
	}

	stored, err := eventStore.Tail(ctx, 500)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	prefix := "schedule:" + info.ID + ":tick:"
	hasToolCall := false
	hasToolResult := false
	for _, ev := range stored {
		if !strings.HasPrefix(ev.RunID, prefix) {
			continue
		}
		if ev.Kind == agentsdk.EventKindToolCall {
			hasToolCall = true
		}
		if ev.Kind == agentsdk.EventKindToolResult {
			hasToolResult = true
		}
	}
	if !hasToolCall || !hasToolResult {
		t.Fatalf("expected scheduled audit tool_call/result, got call=%v result=%v", hasToolCall, hasToolResult)
	}
}

func TestService_SchedulerTick_BindsConversationContext(t *testing.T) {
	client := &schedulerScriptClient{
		responses:    []string{"seed-b-ok"},
		defaultReply: "conv-a-callback",
	}
	ctx, svc, chatStore, _, _ := setupSchedulerServiceTest(t, client)

	if _, err := svc.SendByConversation(ctx, "conv-b", "seed b"); err != nil {
		t.Fatalf("seed conv-b: %v", err)
	}

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		ConversationID: "conv-a",
		IntervalSec:    1,
		TTLSec:         3,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	defer func() { _, _ = svc.CancelSchedule(context.Background(), info.ID) }()

	if strings.TrimSpace(info.ConversationID) != "conv-a" {
		t.Fatalf("conversation_id=%q want %q", info.ConversationID, "conv-a")
	}

	ok := waitForCondition(4*time.Second, func() bool {
		msgs, err := chatStore.TailInConversation(context.Background(), "conv-a", 20)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.Role == chat.RoleAssistant && strings.Contains(m.Content, "conv-a-callback") {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Fatalf("expected callback assistant message in conv-a")
	}

	convBMsgs, err := chatStore.TailInConversation(context.Background(), "conv-b", 20)
	if err != nil {
		t.Fatalf("tail conv-b: %v", err)
	}
	for _, m := range convBMsgs {
		if strings.Contains(m.Content, "conv-a-callback") {
			t.Fatalf("conv-b was polluted by conv-a callback: %+v", convBMsgs)
		}
	}
}

func TestService_SchedulerCallback_DisallowsSchedulerCreate(t *testing.T) {
	client := &schedulerScriptClient{
		responses: []string{
			"<tool_data><call><tool_name>scheduler_create</tool_name><target_tool_name>tasks_count</target_tool_name><tool_fields_json>{}</tool_fields_json></call></tool_data>",
			"回调已继续",
		},
		defaultReply: "回调已继续",
	}
	ctx, svc, chatStore, eventStore, _ := setupSchedulerServiceTest(t, client)

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         3,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	defer func() { _, _ = svc.CancelSchedule(context.Background(), info.ID) }()

	ok := waitForCondition(4*time.Second, func() bool {
		msgs, err := chatStore.Tail(context.Background(), 20)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.Role == chat.RoleAssistant && strings.Contains(m.Content, "回调已继续") {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Fatalf("expected callback assistant reply")
	}

	stored, err := eventStore.Tail(ctx, 800)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	foundCreateToolError := false
	for _, ev := range stored {
		if ev.Kind != agentsdk.EventKindToolResult {
			continue
		}
		if !strings.Contains(ev.EventJSON, `"ToolName":"scheduler_create"`) &&
			!strings.Contains(ev.EventJSON, `"tool_name":"scheduler_create"`) {
			continue
		}
		if (strings.Contains(ev.EventJSON, `"OK":false`) ||
			strings.Contains(strings.ToLower(ev.EventJSON), `"ok":false`)) &&
			strings.Contains(strings.ToLower(ev.EventJSON), "callback") {
			foundCreateToolError = true
			break
		}
	}
	if !foundCreateToolError {
		t.Fatalf("expected scheduler_create tool_result error in callback")
	}
}

func TestService_SchedulerCallback_CanCancelSchedule(t *testing.T) {
	client := &schedulerScriptClient{
		responses: []string{
			"<tool_data><call><tool_name>scheduler_cancel</tool_name><schedule_id>{{SCHEDULE_ID}}</schedule_id></call></tool_data>",
			"已停止该调度",
		},
		defaultReply: "已停止该调度",
	}
	ctx, svc, chatStore, eventStore, _ := setupSchedulerServiceTest(t, client)

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    2,
		TTLSec:         10,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	client.setScheduleID(info.ID)

	ok := waitForCondition(6*time.Second, func() bool { return !scheduleExists(svc, info.ID) })
	if !ok {
		t.Fatalf("expected callback to cancel schedule")
	}

	msgOK := waitForCondition(4*time.Second, func() bool {
		msgs, err := chatStore.Tail(context.Background(), 30)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.Role == chat.RoleAssistant && strings.Contains(m.Content, "已停止该调度") {
				return true
			}
		}
		return false
	})
	if !msgOK {
		t.Fatalf("expected callback to append stop message")
	}

	rawMsgs, err := chatStore.Tail(context.Background(), 50)
	if err != nil {
		t.Fatalf("tail chat raw: %v", err)
	}
	hasToolData := false
	hasToolResult := false
	for _, m := range rawMsgs {
		text := strings.TrimSpace(m.Content)
		if m.Role == chat.RoleAssistant && strings.Contains(text, "<tool_data>") {
			hasToolData = true
		}
		if m.Role == chat.RoleUser && strings.Contains(text, "<tool_result>") {
			hasToolResult = true
		}
	}
	if !hasToolData || !hasToolResult {
		t.Fatalf("expected scheduler callback transcript persisted with tool_data/tool_result messages")
	}

	visibleMsgs, err := svc.History(context.Background(), 50)
	if err != nil {
		t.Fatalf("service history: %v", err)
	}
	for _, m := range visibleMsgs {
		if strings.Contains(m.Content, "<tool_data>") || strings.Contains(m.Content, "<tool_result>") {
			t.Fatalf("expected visible history to hide internal transcript message: %+v", m)
		}
	}

	time.Sleep(2200 * time.Millisecond)

	stored, err := eventStore.Tail(ctx, 1000)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	prefix := "schedule:" + info.ID + ":tick:"
	ticks := map[string]struct{}{}
	for _, ev := range stored {
		if strings.HasPrefix(ev.RunID, prefix) {
			ticks[ev.RunID] = struct{}{}
		}
	}
	if len(ticks) > 1 {
		t.Fatalf("expected schedule canceled after first tick, got tick runs=%d", len(ticks))
	}
}

func TestService_SchedulerCancel_RemovesScheduleFromMemory(t *testing.T) {
	ctx, svc, _, _, _ := setupSchedulerServiceTest(t, &schedulerScriptClient{defaultReply: "ok"})
	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    2,
		TTLSec:         60,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if _, err := svc.CancelSchedule(ctx, info.ID); err != nil {
		t.Fatalf("cancel schedule: %v", err)
	}

	ok := waitForCondition(2*time.Second, func() bool { return !scheduleExists(svc, info.ID) })
	if !ok {
		t.Fatalf("expected canceled schedule to be removed from memory")
	}
}

func TestService_SchedulerExpire_RemovesScheduleFromMemory(t *testing.T) {
	ctx, svc, _, _, _ := setupSchedulerServiceTest(t, &schedulerScriptClient{defaultReply: "ok"})
	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    2,
		TTLSec:         1,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	ok := waitForCondition(5*time.Second, func() bool { return !scheduleExists(svc, info.ID) })
	if !ok {
		t.Fatalf("expected expired schedule to be removed from memory")
	}
}

func TestService_SchedulerEvents_AutoPrunesRuns(t *testing.T) {
	ctx, svc, _, eventStore, _ := setupSchedulerServiceTest(t, &schedulerScriptClient{defaultReply: "ok"})

	for i := 0; i < 260; i++ {
		runID := fmt.Sprintf("schedule:s-%03d:tick:1", i)
		svc.appendScheduleEvent(runID, agentsdk.Event{
			Kind:     agentsdk.EventKindTrace,
			Protocol: "scheduler",
			Step:     0,
			Time:     time.Now().UTC(),
			Payload:  agentsdk.TraceEvent{Message: "x"},
		})
	}

	stored, err := eventStore.Tail(ctx, 2000)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	runs := make(map[string]struct{}, len(stored))
	for _, ev := range stored {
		runs[ev.RunID] = struct{}{}
	}
	if len(runs) > 200 {
		t.Fatalf("expected scheduler event runs to be pruned to <=200, got %d", len(runs))
	}
}

func TestService_SchedulerCallback_DoesNotBlockUserSend(t *testing.T) {
	client := newSchedulerBlockingClient()
	ctx, svc, _, _, _ := setupSchedulerServiceTest(t, client)

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "tasks_count",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         10,
		AllowWrite:     false,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	defer func() {
		client.unblock()
		_, _ = svc.CancelSchedule(context.Background(), info.ID)
	}()

	select {
	case <-client.callbackStarted:
	case <-time.After(4 * time.Second):
		t.Fatalf("timed out waiting for scheduler callback start")
	}

	sendDone := make(chan struct{})
	var sendErr error
	go func() {
		_, sendErr = svc.Send(context.Background(), "hello")
		close(sendDone)
	}()

	select {
	case <-client.sendStarted:
		// User send should proceed even while callback is still running.
	case <-time.After(600 * time.Millisecond):
		t.Fatalf("user send was blocked by scheduler callback")
	}

	client.unblock()
	select {
	case <-sendDone:
	case <-time.After(4 * time.Second):
		t.Fatalf("timed out waiting user send completion")
	}
	if sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}
}

func TestService_SchedulerErrors_ContinueUntilTTL(t *testing.T) {
	client := &schedulerScriptClient{defaultReply: "已记录"}
	ctx, svc, _, eventStore, _ := setupSchedulerServiceTest(t, client)

	info, err := svc.CreateSchedule(ctx, sectools.SchedulerCreateRequest{
		ToolName:       "task_new_submit",
		ToolFieldsJSON: `{}`,
		ToolFields:     map[string]string{},
		IntervalSec:    1,
		TTLSec:         3,
		AllowWrite:     true,
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	defer func() { _, _ = svc.CancelSchedule(context.Background(), info.ID) }()

	ok := waitForCondition(6*time.Second, func() bool {
		list, err := svc.ListSchedules(context.Background())
		if err != nil {
			return false
		}
		for _, item := range list {
			if item.ID == info.ID {
				return false
			}
		}
		return true
	})
	if !ok {
		t.Fatalf("expected schedule to stop after ttl")
	}

	stored, err := eventStore.Tail(ctx, 2000)
	if err != nil {
		t.Fatalf("tail events: %v", err)
	}
	prefix := "schedule:" + info.ID + ":tick:"
	errorToolResults := 0
	for _, ev := range stored {
		if !strings.HasPrefix(ev.RunID, prefix) || ev.Kind != agentsdk.EventKindToolResult {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(ev.EventJSON), &payload); err != nil {
			continue
		}
		rawPayload, _ := payload["Payload"].(map[string]any)
		if rawPayload == nil {
			rawPayload, _ = payload["payload"].(map[string]any)
		}
		toolName := strings.TrimSpace(toString(rawPayload["ToolName"]))
		if toolName == "" {
			toolName = strings.TrimSpace(toString(rawPayload["tool_name"]))
		}
		okVal, _ := rawPayload["OK"].(bool)
		if _, ok := rawPayload["OK"]; !ok {
			okVal, _ = rawPayload["ok"].(bool)
		}
		if toolName == "task_new_submit" && !okVal {
			errorToolResults++
		}
	}
	if errorToolResults < 2 {
		t.Fatalf("expected repeated tool errors before ttl, got %d", errorToolResults)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}
