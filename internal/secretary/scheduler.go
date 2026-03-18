package secretary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/agentsdk/sessioncompress"
	"controlccx/internal/agentsdk/xmlprotocol"
	"controlccx/internal/events"
	"controlccx/internal/secretary/llm"
	sectools "controlccx/internal/secretary/tools"
)

const (
	defaultScheduleIntervalSec = 10
	maxScheduleIntervalSec     = 60
	defaultScheduleTTLSec      = 300
	scheduleCallbackTimeout    = 90 * time.Second
)

var schedulerReadOnlyTools = map[string]struct{}{
	"system_info":    {},
	"fs_roots":       {},
	"fs_pwd":         {},
	"fs_entries":     {},
	"fs_read_text":   {},
	"tasks_count":    {},
	"tasks_list":     {},
	"task_logs_tail": {},
	"task_log_get":   {},
}

type schedulerCallbackContextKey struct{}

type schedulerCallbackContext struct {
	ScheduleID     string
	TickNo         int
	ConversationID string
}

type scheduleJob struct {
	id               string
	targetToolName   string
	targetFields     map[string]string
	targetFieldsJSON string
	conversationID   string

	intervalSec int
	ttlSec      int
	allowWrite  bool

	interval time.Duration
	ttl      time.Duration

	createdAt  time.Time
	expiresAt  time.Time
	nextTickAt time.Time

	state   sectools.ScheduleState
	tickNo  int
	running bool
	pending bool

	expiryReported bool

	ctx    context.Context
	cancel context.CancelFunc
}

type scheduleInvocationResult struct {
	OK         bool
	Output     any
	OutputJSON string
	Error      string
}

func (s *Service) CreateSchedule(ctx context.Context, req sectools.SchedulerCreateRequest) (sectools.ScheduleInfo, error) {
	if s == nil {
		return sectools.ScheduleInfo{}, errors.New("secretary: scheduler service is nil")
	}
	if reqCtx, ok := schedulerContextFrom(ctx); ok && strings.TrimSpace(reqCtx.ScheduleID) != "" {
		return sectools.ScheduleInfo{}, errors.New("scheduler_create is not allowed in scheduler callback context")
	}

	toolName := strings.TrimSpace(req.ToolName)
	if toolName == "" {
		return sectools.ScheduleInfo{}, errors.New("tool_name is required")
	}
	if toolName == "scheduler_create" {
		return sectools.ScheduleInfo{}, errors.New("scheduler_create cannot target scheduler_create")
	}
	if !scheduleToolExists(toolName) {
		return sectools.ScheduleInfo{}, fmt.Errorf("unknown tool: %s", toolName)
	}

	intervalSec := req.IntervalSec
	if intervalSec <= 0 {
		intervalSec = defaultScheduleIntervalSec
	}
	if intervalSec > maxScheduleIntervalSec {
		return sectools.ScheduleInfo{}, fmt.Errorf("interval_sec must be <= %d", maxScheduleIntervalSec)
	}

	ttlSec := req.TTLSec
	if ttlSec <= 0 {
		ttlSec = defaultScheduleTTLSec
	}

	allowWrite := req.AllowWrite
	if !allowWrite {
		if _, ok := schedulerReadOnlyTools[toolName]; !ok {
			return sectools.ScheduleInfo{}, fmt.Errorf("tool %q is write-capable and requires allow_write=true", toolName)
		}
	}

	fields := make(map[string]string, len(req.ToolFields))
	for k, v := range req.ToolFields {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		fields[key] = strings.TrimSpace(v)
	}
	fieldsJSON := strings.TrimSpace(req.ToolFieldsJSON)
	if fieldsJSON == "" {
		b, err := json.Marshal(fields)
		if err != nil {
			return sectools.ScheduleInfo{}, fmt.Errorf("scheduler: marshal tool fields: %w", err)
		}
		fieldsJSON = string(b)
	}
	conversationID := s.resolveScheduleConversationID(ctx, req, fields)

	now := time.Now().UTC()
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &scheduleJob{
		id:               newRunID(),
		targetToolName:   toolName,
		targetFields:     fields,
		targetFieldsJSON: fieldsJSON,
		conversationID:   conversationID,
		intervalSec:      intervalSec,
		ttlSec:           ttlSec,
		allowWrite:       allowWrite,
		interval:         time.Duration(intervalSec) * time.Second,
		ttl:              time.Duration(ttlSec) * time.Second,
		createdAt:        now,
		expiresAt:        now.Add(time.Duration(ttlSec) * time.Second),
		nextTickAt:       now.Add(time.Duration(intervalSec) * time.Second),
		state:            sectools.ScheduleStateActive,
		ctx:              jobCtx,
		cancel:           cancel,
	}

	s.scheduleMu.Lock()
	if s.schedules == nil {
		s.schedules = make(map[string]*scheduleJob)
	}
	s.schedules[job.id] = job
	info := s.snapshotSchedule(job)
	s.scheduleMu.Unlock()

	go s.runScheduleLoop(job)
	return info, nil
}

func (s *Service) ListSchedules(ctx context.Context) ([]sectools.ScheduleInfo, error) {
	_ = ctx
	if s == nil {
		return nil, errors.New("secretary: scheduler service is nil")
	}
	s.scheduleMu.Lock()
	list := make([]sectools.ScheduleInfo, 0, len(s.schedules))
	for _, job := range s.schedules {
		if job == nil || job.state != sectools.ScheduleStateActive {
			continue
		}
		list = append(list, s.snapshotSchedule(job))
	}
	s.scheduleMu.Unlock()
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].ID < list[j].ID
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

func (s *Service) CancelSchedule(ctx context.Context, scheduleID string) (sectools.ScheduleInfo, error) {
	_ = ctx
	if s == nil {
		return sectools.ScheduleInfo{}, errors.New("secretary: scheduler service is nil")
	}
	id := strings.TrimSpace(scheduleID)
	if id == "" {
		return sectools.ScheduleInfo{}, errors.New("schedule_id is required")
	}

	var cancel context.CancelFunc
	s.scheduleMu.Lock()
	job, ok := s.schedules[id]
	if !ok || job == nil {
		s.scheduleMu.Unlock()
		return sectools.ScheduleInfo{}, fmt.Errorf("schedule not found: %s", id)
	}
	if job.state == sectools.ScheduleStateActive {
		job.state = sectools.ScheduleStateCanceled
		job.pending = false
		job.nextTickAt = time.Time{}
		cancel = job.cancel
	}
	s.removeScheduleIfTerminalLocked(job)
	info := s.snapshotSchedule(job)
	s.scheduleMu.Unlock()

	if cancel != nil {
		cancel()
	}
	return info, nil
}

func (s *Service) runScheduleLoop(job *scheduleJob) {
	if s == nil || job == nil {
		return
	}

	ticker := time.NewTicker(job.interval)
	defer ticker.Stop()

	ttlWait := time.Until(job.expiresAt)
	if ttlWait < 0 {
		ttlWait = 0
	}
	expiryTimer := time.NewTimer(ttlWait)
	defer expiryTimer.Stop()

	for {
		select {
		case <-job.ctx.Done():
			return
		case <-expiryTimer.C:
			s.expireSchedule(job, "ttl_reached")
			return
		case <-ticker.C:
			s.requestScheduleTick(job)
		}
	}
}

func (s *Service) requestScheduleTick(job *scheduleJob) {
	if s == nil || job == nil {
		return
	}

	now := time.Now().UTC()
	var tickNo int
	s.scheduleMu.Lock()
	if job.state != sectools.ScheduleStateActive {
		s.scheduleMu.Unlock()
		return
	}
	if !now.Before(job.expiresAt) {
		s.scheduleMu.Unlock()
		s.expireSchedule(job, "ttl_reached")
		return
	}
	if job.running {
		job.pending = true
		s.scheduleMu.Unlock()
		return
	}
	job.running = true
	job.tickNo++
	tickNo = job.tickNo
	job.nextTickAt = now.Add(job.interval)
	s.scheduleMu.Unlock()

	go s.executeScheduleTick(job, tickNo)
}

func (s *Service) executeScheduleTick(job *scheduleJob, tickNo int) {
	if s == nil || job == nil {
		return
	}
	runID := scheduleTickRunID(job.id, tickNo)
	result := s.executeScheduledTool(job, tickNo, runID)
	s.runScheduleCallback(job, tickNo, runID, job.targetToolName, result, "")
	s.finishScheduleTick(job)
}

func (s *Service) executeScheduledTool(job *scheduleJob, tickNo int, runID string) scheduleInvocationResult {
	callID := scheduleToolCallID(job.id, tickNo)
	toolName := strings.TrimSpace(job.targetToolName)

	s.appendScheduleEvent(runID, agentsdk.Event{
		Kind:     agentsdk.EventKindToolCall,
		Protocol: "scheduler",
		Step:     0,
		Time:     time.Now().UTC(),
		Payload: agentsdk.ToolCallEvent{
			ID:     callID,
			Name:   toolName,
			Fields: cloneFields(job.targetFields),
			Raw:    "",
		},
	})
	s.publishSecretaryThinking(job.id, tickNo, map[string]any{
		"source":           "timer",
		"schedule_id":      job.id,
		"tick_no":          tickNo,
		"conversation_id":  strings.TrimSpace(job.conversationID),
		"target_tool_name": toolName,
		"kind":             "tool_call",
		"tool_name":        toolName,
		"line":             fmt.Sprintf("定时调用工具：%s", toolName),
	})

	reg := sectools.NewRegistry(s.toolDeps(runID))
	payload, toolErr := reg.Execute(job.ctx, agentsdk.ToolCall{
		ID:     callID,
		Name:   toolName,
		Fields: cloneFields(job.targetFields),
		Raw:    "",
	})
	if toolErr != nil {
		payload = map[string]string{"error": fmt.Sprintf("Tool execution failed: %v", toolErr)}
	}

	outputJSON, errMsg := marshalToolOutput(payload, toolErr)
	ok := strings.TrimSpace(errMsg) == ""

	s.appendScheduleEvent(runID, agentsdk.Event{
		Kind:     agentsdk.EventKindToolResult,
		Protocol: "scheduler",
		Step:     0,
		Time:     time.Now().UTC(),
		Payload: agentsdk.ToolResultEvent{
			ToolName:   toolName,
			ToolCallID: callID,
			OK:         ok,
			OutputJSON: outputJSON,
			Error:      strings.TrimSpace(errMsg),
		},
	})

	line := fmt.Sprintf("工具完成：%s", toolName)
	if !ok {
		if strings.TrimSpace(errMsg) != "" {
			line = fmt.Sprintf("工具失败：%s（%s）", toolName, strings.TrimSpace(errMsg))
		} else {
			line = fmt.Sprintf("工具失败：%s", toolName)
		}
	}
	s.publishSecretaryThinking(job.id, tickNo, map[string]any{
		"source":           "timer",
		"schedule_id":      job.id,
		"tick_no":          tickNo,
		"conversation_id":  strings.TrimSpace(job.conversationID),
		"target_tool_name": toolName,
		"kind":             "tool_result",
		"tool_name":        toolName,
		"ok":               ok,
		"error":            strings.TrimSpace(errMsg),
		"line":             line,
	})

	return scheduleInvocationResult{
		OK:         ok,
		Output:     payload,
		OutputJSON: outputJSON,
		Error:      strings.TrimSpace(errMsg),
	}
}

func (s *Service) runScheduleCallback(job *scheduleJob, tickNo int, runID string, targetToolName string, result scheduleInvocationResult, reason string) {
	if s == nil || job == nil {
		return
	}

	callbackBase, cancel := context.WithTimeout(context.Background(), scheduleCallbackTimeout)
	defer cancel()

	callbackCtx := context.WithValue(callbackBase, schedulerCallbackContextKey{}, schedulerCallbackContext{
		ScheduleID:     job.id,
		TickNo:         tickNo,
		ConversationID: strings.TrimSpace(job.conversationID),
	})

	payload := map[string]any{
		"source":           "timer",
		"schedule_id":      job.id,
		"tick_no":          tickNo,
		"conversation_id":  strings.TrimSpace(job.conversationID),
		"target_tool_name": strings.TrimSpace(targetToolName),
		"tool_result": map[string]any{
			"ok":     result.OK,
			"output": result.Output,
			"error":  strings.TrimSpace(result.Error),
		},
	}
	if strings.TrimSpace(reason) != "" {
		payload["reason"] = strings.TrimSpace(reason)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		payloadJSON = []byte(`{"source":"timer","error":"payload marshal failed"}`)
	}

	toolResultMessage := buildSingleToolResultMessage(
		strings.TrimSpace(targetToolName),
		scheduleToolCallID(job.id, tickNo),
		scheduleInvocationResult{
			OK:         result.OK,
			OutputJSON: string(payloadJSON),
			Error:      strings.TrimSpace(result.Error),
		},
	)

	client := s.client
	if client == nil {
		backend := llm.NewProviderBackendWithProviders(s.cfg, s.auth, s.providers)
		client = &llm.Client{Backend: backend}
	}

	history, err := s.promptHistory(callbackCtx, client, runID, job.conversationID)
	if err != nil {
		s.publishSecretaryThinking(job.id, tickNo, map[string]any{
			"source":          "timer",
			"schedule_id":     job.id,
			"tick_no":         tickNo,
			"conversation_id": strings.TrimSpace(job.conversationID),
			"kind":            "error",
			"error":           err.Error(),
			"line":            "定时回调构建上下文失败：" + strings.TrimSpace(err.Error()),
		})
		return
	}

	messages := make([]agentsdk.Message, 0, 1+len(history)+1)
	messages = append(messages, agentsdk.Message{Role: "system", Content: buildSystemPrompt()})
	messages = append(messages, history...)
	messages = append(messages, agentsdk.Message{Role: "user", Content: toolResultMessage})

	contextCompressThreshold := s.compressOpts.MaxContextRunes
	if contextCompressThreshold < sessioncompress.DefaultMaxContextRunes {
		contextCompressThreshold = sessioncompress.DefaultMaxContextRunes
	}

	stepRecords := make([]xmlprotocol.StepRecord, 0, 8)
	finalAssistantContent := ""
	out, runErr := xmlprotocol.RunLoop(callbackCtx, xmlprotocol.RunLoopInput{
		Client:                        client,
		Messages:                      messages,
		LLMOptions:                    s.llmOptionsBestEffort(callbackCtx),
		ContextCompressThresholdRunes: contextCompressThreshold,
		Executor:                      sectools.NewRegistry(s.toolDeps(runID)),
		MaxSteps:                      500,
		Callbacks: xmlprotocol.Callbacks{
			EventSink: s.scheduleCallbackEventSink(runID, job.id, tickNo),
			ObserveStep: func(record xmlprotocol.StepRecord) {
				stepRecords = append(stepRecords, record)
			},
			ObserveFinal: func(visibleContent, assistantContent string) {
				_ = visibleContent
				finalAssistantContent = strings.TrimSpace(assistantContent)
			},
		},
	})

	reply := strings.TrimSpace(out)
	if runErr != nil {
		s.publishSecretaryThinking(job.id, tickNo, map[string]any{
			"source":          "timer",
			"schedule_id":     job.id,
			"tick_no":         tickNo,
			"conversation_id": strings.TrimSpace(job.conversationID),
			"kind":            "error",
			"error":           strings.TrimSpace(runErr.Error()),
			"line":            "定时回调执行失败：" + strings.TrimSpace(runErr.Error()),
		})
	}
	if reply == "" {
		return
	}

	s.sendMu.Lock()
	if err := s.appendRunLoopTranscript(callbackCtx, job.conversationID, toolResultMessage, stepRecords, finalAssistantContent, reply); err != nil {
		s.sendMu.Unlock()
		s.publishSecretaryThinking(job.id, tickNo, map[string]any{
			"source":          "timer",
			"schedule_id":     job.id,
			"tick_no":         tickNo,
			"conversation_id": strings.TrimSpace(job.conversationID),
			"kind":            "error",
			"error":           strings.TrimSpace(err.Error()),
			"line":            "定时回调写入聊天失败：" + strings.TrimSpace(err.Error()),
		})
		return
	}
	_ = s.chat.PruneKeepLastInConversation(callbackCtx, job.conversationID, 2000)
	s.sendMu.Unlock()

	s.publishSecretaryMessage(map[string]any{
		"source":          "timer",
		"schedule_id":     job.id,
		"tick_no":         tickNo,
		"conversation_id": strings.TrimSpace(job.conversationID),
		"role":            "assistant",
		"content":         reply,
		"time":            time.Now().UTC(),
	})
}

func (s *Service) scheduleCallbackEventSink(runID string, scheduleID string, tickNo int) agentsdk.EventSink {
	return agentsdk.EventSinkFunc(func(ctx context.Context, ev agentsdk.Event) {
		_ = ctx
		s.appendScheduleEvent(runID, ev)

		payload := map[string]any{
			"source":      "timer",
			"phase":       "callback",
			"schedule_id": scheduleID,
			"tick_no":     tickNo,
		}

		switch ev.Kind {
		case agentsdk.EventKindTrace:
			trace, _ := ev.Payload.(agentsdk.TraceEvent)
			line := strings.TrimSpace(trace.Message)
			if line == "" {
				return
			}
			payload["kind"] = "trace"
			payload["step"] = ev.Step
			payload["line"] = line
			s.publishSecretaryThinking(scheduleID, tickNo, payload)
		case agentsdk.EventKindToolCall:
			call, _ := ev.Payload.(agentsdk.ToolCallEvent)
			name := strings.TrimSpace(call.Name)
			if name == "" {
				name = "unknown"
			}
			payload["kind"] = "tool_call"
			payload["step"] = ev.Step
			payload["tool_name"] = name
			payload["line"] = fmt.Sprintf("回调调用工具：%s", name)
			s.publishSecretaryThinking(scheduleID, tickNo, payload)
		case agentsdk.EventKindToolResult:
			res, _ := ev.Payload.(agentsdk.ToolResultEvent)
			name := strings.TrimSpace(res.ToolName)
			if name == "" {
				name = "unknown"
			}
			line := fmt.Sprintf("回调工具完成：%s", name)
			if !res.OK {
				if strings.TrimSpace(res.Error) != "" {
					line = fmt.Sprintf("回调工具失败：%s（%s）", name, strings.TrimSpace(res.Error))
				} else {
					line = fmt.Sprintf("回调工具失败：%s", name)
				}
			}
			payload["kind"] = "tool_result"
			payload["step"] = ev.Step
			payload["tool_name"] = name
			payload["ok"] = res.OK
			payload["error"] = strings.TrimSpace(res.Error)
			payload["line"] = line
			s.publishSecretaryThinking(scheduleID, tickNo, payload)
		case agentsdk.EventKindError:
			errEvt, _ := ev.Payload.(agentsdk.ErrorEvent)
			line := strings.TrimSpace(errEvt.Error)
			if line == "" {
				return
			}
			payload["kind"] = "error"
			payload["step"] = ev.Step
			payload["error"] = line
			payload["line"] = line
			s.publishSecretaryThinking(scheduleID, tickNo, payload)
		}
	})
}

func (s *Service) finishScheduleTick(job *scheduleJob) {
	if s == nil || job == nil {
		return
	}

	now := time.Now().UTC()
	var rerunTickNo int
	needRerun := false
	needExpire := false

	s.scheduleMu.Lock()
	switch job.state {
	case sectools.ScheduleStateActive:
		job.running = false
		if !now.Before(job.expiresAt) {
			needExpire = true
		} else if job.pending {
			job.pending = false
			job.running = true
			job.tickNo++
			rerunTickNo = job.tickNo
			job.nextTickAt = now.Add(job.interval)
			needRerun = true
		}
	default:
		job.running = false
		job.pending = false
		job.nextTickAt = time.Time{}
	}
	s.removeScheduleIfTerminalLocked(job)
	s.scheduleMu.Unlock()

	if needExpire {
		s.expireSchedule(job, "ttl_reached")
		return
	}
	if needRerun {
		go s.executeScheduleTick(job, rerunTickNo)
	}
}

func (s *Service) expireSchedule(job *scheduleJob, reason string) {
	if s == nil || job == nil {
		return
	}

	var (
		shouldNotify bool
		tickNo       int
		cancel       context.CancelFunc
	)

	s.scheduleMu.Lock()
	if job.state == sectools.ScheduleStateActive {
		job.state = sectools.ScheduleStateExpired
		job.pending = false
		job.nextTickAt = time.Time{}
		cancel = job.cancel
		if !job.expiryReported {
			job.expiryReported = true
			job.tickNo++
			tickNo = job.tickNo
			shouldNotify = true
		}
	}
	s.scheduleMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !shouldNotify {
		s.cleanupScheduleIfTerminal(job)
		return
	}

	runID := scheduleTickRunID(job.id, tickNo)
	errText := "schedule expired: ttl reached"
	if strings.TrimSpace(reason) != "" {
		errText = "schedule expired: " + strings.TrimSpace(reason)
	}

	s.appendScheduleEvent(runID, agentsdk.Event{
		Kind:     agentsdk.EventKindToolResult,
		Protocol: "scheduler",
		Step:     0,
		Time:     time.Now().UTC(),
		Payload: agentsdk.ToolResultEvent{
			ToolName:   "scheduler_expire",
			ToolCallID: scheduleToolCallID(job.id, tickNo),
			OK:         false,
			OutputJSON: `{"expired":true}`,
			Error:      errText,
		},
	})
	s.publishSecretaryThinking(job.id, tickNo, map[string]any{
		"source":          "timer",
		"schedule_id":     job.id,
		"tick_no":         tickNo,
		"conversation_id": strings.TrimSpace(job.conversationID),
		"kind":            "tool_result",
		"tool_name":       "scheduler_expire",
		"ok":              false,
		"error":           errText,
		"line":            "调度已到期并自动停止",
	})

	s.runScheduleCallback(job, tickNo, runID, "scheduler_expire", scheduleInvocationResult{
		OK:         false,
		Output:     map[string]any{"expired": true, "reason": strings.TrimSpace(reason)},
		OutputJSON: `{"expired":true}`,
		Error:      errText,
	}, "ttl_expired")
	s.cleanupScheduleIfTerminal(job)
}

func (s *Service) snapshotSchedule(job *scheduleJob) sectools.ScheduleInfo {
	if job == nil {
		return sectools.ScheduleInfo{}
	}
	return sectools.ScheduleInfo{
		ID:               strings.TrimSpace(job.id),
		TargetToolName:   strings.TrimSpace(job.targetToolName),
		TargetFieldsJSON: strings.TrimSpace(job.targetFieldsJSON),
		ConversationID:   strings.TrimSpace(job.conversationID),
		IntervalSec:      job.intervalSec,
		TTLSec:           job.ttlSec,
		AllowWrite:       job.allowWrite,
		State:            job.state,
		CreatedAt:        job.createdAt,
		ExpiresAt:        job.expiresAt,
		NextTickAt:       job.nextTickAt,
		TickNo:           job.tickNo,
		Running:          job.running,
		Pending:          job.pending,
	}
}

func (s *Service) resolveScheduleConversationID(ctx context.Context, req sectools.SchedulerCreateRequest, fields map[string]string) string {
	if v := strings.TrimSpace(req.ConversationID); v != "" {
		return normalizeConversationID(v)
	}
	if meta, ok := schedulerContextFrom(ctx); ok {
		if v := strings.TrimSpace(meta.ConversationID); v != "" {
			return normalizeConversationID(v)
		}
	}
	if v := strings.TrimSpace(fields["conversation_id"]); v != "" {
		return normalizeConversationID(v)
	}
	taskID := strings.TrimSpace(fields["task_id"])
	if s == nil || s.tasks == nil || taskID == "" {
		return normalizeConversationID("")
	}
	taskCtx := ctx
	if taskCtx == nil {
		taskCtx = context.Background()
	}
	task, err := s.tasks.GetTask(taskCtx, taskID)
	if err != nil {
		return normalizeConversationID("")
	}
	return normalizeConversationID(task.ConversationID)
}

func (s *Service) appendScheduleEvent(runID string, ev agentsdk.Event) {
	if s == nil || s.events == nil {
		return
	}
	id := strings.TrimSpace(runID)
	if id == "" {
		return
	}
	_ = s.events.Append(context.Background(), id, ev)
	s.maybePruneEvents(context.Background(), false)
}

func (s *Service) removeScheduleIfTerminalLocked(job *scheduleJob) {
	if s == nil || job == nil {
		return
	}
	if job.state == sectools.ScheduleStateActive || job.running {
		return
	}
	if s.schedules == nil {
		return
	}
	delete(s.schedules, strings.TrimSpace(job.id))
}

func (s *Service) cleanupScheduleIfTerminal(job *scheduleJob) {
	if s == nil || job == nil {
		return
	}
	s.scheduleMu.Lock()
	s.removeScheduleIfTerminalLocked(job)
	s.scheduleMu.Unlock()
}

func (s *Service) publishSecretaryThinking(scheduleID string, tickNo int, payload map[string]any) {
	if s == nil || s.hub == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["source"]; !ok {
		payload["source"] = "timer"
	}
	if _, ok := payload["schedule_id"]; !ok {
		payload["schedule_id"] = strings.TrimSpace(scheduleID)
	}
	if _, ok := payload["tick_no"]; !ok {
		payload["tick_no"] = tickNo
	}
	s.hub.Publish(events.Event{Type: "secretary.thinking", Time: time.Now().UTC(), Payload: payload})
}

func (s *Service) publishSecretaryMessage(payload map[string]any) {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.Publish(events.Event{Type: "secretary.message", Time: time.Now().UTC(), Payload: payload})
}

func scheduleTickRunID(scheduleID string, tickNo int) string {
	return fmt.Sprintf("schedule:%s:tick:%d", strings.TrimSpace(scheduleID), tickNo)
}

func scheduleToolCallID(scheduleID string, tickNo int) string {
	return fmt.Sprintf("schedule_%s_tick_%d", strings.TrimSpace(scheduleID), tickNo)
}

func schedulerContextFrom(ctx context.Context) (schedulerCallbackContext, bool) {
	if ctx == nil {
		return schedulerCallbackContext{}, false
	}
	v := ctx.Value(schedulerCallbackContextKey{})
	meta, ok := v.(schedulerCallbackContext)
	if !ok {
		return schedulerCallbackContext{}, false
	}
	return meta, true
}

func scheduleToolExists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, d := range sectools.Descriptors() {
		if strings.TrimSpace(d.Name) == name {
			return true
		}
	}
	return false
}

func marshalToolOutput(payload any, toolErr error) (string, string) {
	response, marshalErr := json.Marshal(payload)
	errMsg := ""
	if toolErr != nil {
		errMsg = strings.TrimSpace(toolErr.Error())
	}
	if marshalErr != nil {
		if errMsg == "" {
			errMsg = marshalErr.Error()
		} else {
			errMsg = errMsg + "; " + marshalErr.Error()
		}
		response = []byte(fmt.Sprintf(`{"error":"failed to marshal tool response: %v"}`, marshalErr))
	}
	return string(response), strings.TrimSpace(errMsg)
}

func buildSingleToolResultMessage(toolName, toolCallID string, result scheduleInvocationResult) string {
	var b strings.Builder
	b.WriteString("<tool_result>\n")
	b.WriteString("  <call>\n")
	b.WriteString("    <tool_name>")
	b.WriteString(escapeXMLText(strings.TrimSpace(toolName)))
	b.WriteString("</tool_name>\n")
	b.WriteString("    <tool_call_id>")
	b.WriteString(escapeXMLText(strings.TrimSpace(toolCallID)))
	b.WriteString("</tool_call_id>\n")
	b.WriteString("    <ok>")
	if result.OK {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString("</ok>\n")
	if strings.TrimSpace(result.Error) != "" {
		b.WriteString("    <error>")
		b.WriteString(escapeXMLText(strings.TrimSpace(result.Error)))
		b.WriteString("</error>\n")
	}
	if strings.TrimSpace(result.OutputJSON) != "" {
		b.WriteString("    <output>")
		b.WriteString(escapeXMLText(strings.TrimSpace(result.OutputJSON)))
		b.WriteString("</output>\n")
	}
	b.WriteString("  </call>\n")
	b.WriteString("</tool_result>\n")
	return b.String()
}

func escapeXMLText(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
	)
	return replacer.Replace(value)
}

func cloneFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}
