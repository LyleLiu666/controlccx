package taskops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/events"
	"controlccx/internal/runsafe"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
)

type Runner interface {
	Start(ctx context.Context, taskID string) error
	Cancel(ctx context.Context, taskID string) (bool, error)
}

type approvalForwarder interface {
	SubmitApprovalDecision(ctx context.Context, taskID string, approvalID string, decision string, reason string) error
}

type Service struct {
	Tasks   *tasks.Store
	Workers Runner
	Hub     *events.Hub
	Tools   *tooling.Service
}

type RunOptions struct {
	Prompt                string            `json:"prompt"`
	UnsafeAutomation      bool              `json:"unsafe_automation,omitempty"`
	SafetyEnvelope        string            `json:"safety_envelope,omitempty"`
	SafetyPreset          string            `json:"safety_preset,omitempty"`
	TaskIntent            string            `json:"task_intent,omitempty"`
	NetworkTier           tasks.NetworkTier `json:"network_tier,omitempty"`
	CodexSandbox          string            `json:"codex_sandbox,omitempty"`
	CodexApprovalPolicy   string            `json:"codex_approval_policy,omitempty"`
	CodexSearch           bool              `json:"codex_search,omitempty"`
	ClaudePermissionMode  string            `json:"claude_permission_mode,omitempty"`
	ClaudeSandbox         bool              `json:"claude_sandbox,omitempty"`
	ClaudeWebFetchDomains []string          `json:"claude_webfetch_domains,omitempty"`
}

type ContinueResult struct {
	Task  *tasks.Task
	Queue *QueueAck
}

type QueueAck struct {
	Queued          bool   `json:"queued"`
	QueueID         string `json:"queue_id"`
	Position        int    `json:"position"`
	ExistingTaskID  string `json:"existing_task_id,omitempty"`
	ExistingStatus  string `json:"existing_status,omitempty"`
	PreemptedTaskID string `json:"preempted_task_id,omitempty"`
}

type RunnerUnavailableError struct {
	TaskID string
	Err    error
}

func (e *RunnerUnavailableError) Error() string {
	if e == nil || e.Err == nil {
		return "runner unavailable"
	}
	return e.Err.Error()
}

func (s *Service) ContinueSession(ctx context.Context, key string, body RunOptions) (ContinueResult, error) {
	if s == nil || s.Tasks == nil {
		return ContinueResult{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	conversationID, runs, latest, err := s.resolveContinueContext(ctx, key)
	if err != nil {
		return ContinueResult{}, err
	}
	if latest.Status == tasks.StatusBlocked {
		return ContinueResult{}, newMutationError(409, MutationErrorUnsupported, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", "", map[string]any{
			"existing_task_id": strings.TrimSpace(latest.ID),
			"existing_status":  strings.TrimSpace(string(latest.Status)),
		}, nil)
	}

	if inFlight, ok := findInFlightRun(runs); ok {
		ack, err := s.enqueueSessionContinue(ctx, conversationID, body, 0, "continue", &inFlight, "")
		if err != nil {
			return ContinueResult{}, err
		}
		return ContinueResult{Queue: &ack}, nil
	}

	newTask, err := s.createContinueTask(ctx, conversationID, latest, body)
	if err != nil {
		return ContinueResult{}, err
	}
	return ContinueResult{Task: &newTask}, nil
}

func (s *Service) PreemptContinueSession(ctx context.Context, key string, body RunOptions) (QueueAck, error) {
	if s == nil || s.Tasks == nil {
		return QueueAck{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	conversationID, runs, latest, err := s.resolveContinueContext(ctx, key)
	if err != nil {
		return QueueAck{}, err
	}
	if latest.Status == tasks.StatusBlocked {
		return QueueAck{}, newMutationError(409, MutationErrorUnsupported, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", "", map[string]any{
			"existing_task_id": strings.TrimSpace(latest.ID),
			"existing_status":  strings.TrimSpace(string(latest.Status)),
		}, nil)
	}

	preemptedTaskID := ""
	if inFlight, ok := findInFlightRun(runs); ok {
		preemptedTaskID = strings.TrimSpace(inFlight.ID)
		if s.Workers != nil {
			okCancel, err := s.Workers.Cancel(ctx, inFlight.ID)
			if err != nil {
				return QueueAck{}, &RunnerUnavailableError{TaskID: inFlight.ID, Err: err}
			}
			if !okCancel && (inFlight.Status == tasks.StatusQueued || inFlight.Status == tasks.StatusWaiting || inFlight.Status == tasks.StatusAwaitingApproval) {
				_ = s.Tasks.SetCanceled(ctx, inFlight.ID)
				if s.Hub != nil {
					if updated, err := s.Tasks.GetTask(ctx, inFlight.ID); err == nil {
						s.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: updated})
					}
				}
			}
		}
	}

	ack, err := s.enqueueSessionContinue(ctx, conversationID, body, 100, "preempt", nil, preemptedTaskID)
	if err != nil {
		return QueueAck{}, err
	}
	return ack, nil
}

func (s *Service) SessionContinueQueue(ctx context.Context, key string, limit int) ([]tasks.SessionContinueQueueItem, error) {
	if s == nil || s.Tasks == nil {
		return nil, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	conversationID, err := resolveConversationIDForSessionKey(ctx, s.Tasks, key)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return s.Tasks.ListSessionContinueQueueByConversation(ctx, conversationID, limit)
}

func (s *Service) ResumeTask(ctx context.Context, id string, body RunOptions) (tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return tasks.Task{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task id is required", "", nil, nil)
	}

	prev, err := s.Tasks.GetTask(ctx, id)
	if err != nil {
		return tasks.Task{}, err
	}
	if s.Tools != nil {
		if _, ok := s.Tools.Resolve(string(prev.WorkerType)); !ok {
			return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "unknown tool id: "+string(prev.WorkerType), "", nil, nil)
		}
	}
	if strings.TrimSpace(prev.SessionID) == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task has no session_id to resume", "", nil, nil)
	}

	all, err := s.Tasks.ListTasksWithOptions(ctx, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return tasks.Task{}, err
	}
	sid := strings.TrimSpace(prev.SessionID)
	for _, t := range all {
		if t.ID == prev.ID {
			continue
		}
		if strings.TrimSpace(t.SessionID) != sid {
			continue
		}
		if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued || t.Status == tasks.StatusWaiting || t.Status == tasks.StatusAwaitingApproval {
			return tasks.Task{}, fmt.Errorf("session_task_in_flight: existing_task_id=%s existing_status=%s", strings.TrimSpace(t.ID), string(t.Status))
		}
	}

	explicitSafety := body.UnsafeAutomation ||
		strings.TrimSpace(body.SafetyPreset) != "" ||
		strings.TrimSpace(body.TaskIntent) != "" ||
		strings.TrimSpace(string(body.NetworkTier)) != "" ||
		strings.TrimSpace(body.CodexSandbox) != "" ||
		strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
		body.CodexSearch ||
		strings.TrimSpace(body.ClaudePermissionMode) != "" ||
		body.ClaudeSandbox ||
		len(body.ClaudeWebFetchDomains) > 0

	unsafe := body.UnsafeAutomation
	safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
	safetyPreset := strings.TrimSpace(body.SafetyPreset)
	taskIntent := strings.TrimSpace(body.TaskIntent)
	networkTier := body.NetworkTier
	codexSandbox := strings.TrimSpace(body.CodexSandbox)
	codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
	codexSearch := body.CodexSearch
	claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
	claudeSandbox := body.ClaudeSandbox
	claudeDomains := body.ClaudeWebFetchDomains

	if !explicitSafety {
		unsafe = prev.UnsafeAutomation
		safetyPreset = strings.TrimSpace(prev.SafetyPreset)
		taskIntent = strings.TrimSpace(prev.TaskIntent)
		networkTier = prev.NetworkTier
		codexSandbox = strings.TrimSpace(prev.CodexSandbox)
		codexApprovalPolicy = strings.TrimSpace(prev.CodexApprovalPolicy)
		codexSearch = prev.CodexSearch
		claudePermissionMode = strings.TrimSpace(prev.ClaudePermissionMode)
		claudeSandbox = prev.ClaudeSandbox
		claudeDomains = append([]string{}, prev.ClaudeWebFetchDomains...)
	}

	resumeIn := tasks.CreateTaskInput{
		WorkerType:            prev.WorkerType,
		Mode:                  tasks.ModeResume,
		ConversationID:        prev.ConversationID,
		UnsafeAutomation:      unsafe,
		SafetyEnvelope:        safetyEnvelope,
		SafetyPreset:          safetyPreset,
		TaskIntent:            taskIntent,
		NetworkTier:           networkTier,
		CodexSandbox:          codexSandbox,
		CodexApprovalPolicy:   codexApprovalPolicy,
		CodexSearch:           codexSearch,
		ClaudePermissionMode:  claudePermissionMode,
		ClaudeSandbox:         claudeSandbox,
		ClaudeWebFetchDomains: claudeDomains,
		Prompt:                body.Prompt,
		WorkDir:               prev.WorkDir,
		SessionID:             prev.SessionID,
		Warning:               prev.Warning,
	}

	driver := prev.WorkerType
	if s.Tools != nil {
		if profile, ok := s.Tools.Resolve(string(prev.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
			driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
		}
	}
	envelope := runsafe.SafetyEnvelope(strings.TrimSpace(resumeIn.SafetyEnvelope))
	resumeIn, ap := runsafe.ApplyAutopilot(ctx, resumeIn, runsafe.ApplyOptions{
		Driver:   driver,
		Envelope: envelope,
		Classify: runsafe.ClassifyOptions{},
	})

	newTask, err := s.Tasks.CreateTask(ctx, resumeIn)
	if err != nil {
		return tasks.Task{}, err
	}
	if ap.Applied {
		if audit := runsafe.FormatAuditLog(driver, ap.Decision, resumeIn, true); strings.TrimSpace(audit) != "" {
			_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
		}
	}

	return s.startTask(ctx, newTask)
}

func (s *Service) RehydrateTask(ctx context.Context, id string, body RunOptions) (tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return tasks.Task{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task id is required", "", nil, nil)
	}

	src, err := s.Tasks.GetTask(ctx, id)
	if err != nil {
		return tasks.Task{}, err
	}
	if s.Tools != nil {
		if _, ok := s.Tools.Resolve(string(src.WorkerType)); !ok {
			return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "unknown tool id: "+string(src.WorkerType), "", nil, nil)
		}
	}
	if strings.TrimSpace(src.SessionID) == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task has no session_id to rehydrate", "", nil, nil)
	}

	driver := src.WorkerType
	if s.Tools != nil {
		if profile, ok := s.Tools.Resolve(string(src.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
			driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
		}
	}
	if driver != tasks.WorkerClaudeCode {
		return tasks.Task{}, newMutationError(400, MutationErrorUnsupported, "rehydrate is only supported for claude-code sessions", "", nil, nil)
	}

	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = "continue"
	}
	ctxPrompt, err := tasks.BuildRehydratePrompt(ctx, s.Tasks, strings.TrimSpace(src.ConversationID), prompt)
	if err != nil {
		return tasks.Task{}, err
	}

	explicitSafety := body.UnsafeAutomation ||
		strings.TrimSpace(body.SafetyPreset) != "" ||
		strings.TrimSpace(body.TaskIntent) != "" ||
		strings.TrimSpace(string(body.NetworkTier)) != "" ||
		strings.TrimSpace(body.CodexSandbox) != "" ||
		strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
		body.CodexSearch ||
		strings.TrimSpace(body.ClaudePermissionMode) != "" ||
		body.ClaudeSandbox ||
		len(body.ClaudeWebFetchDomains) > 0

	unsafe := body.UnsafeAutomation
	safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
	safetyPreset := strings.TrimSpace(body.SafetyPreset)
	taskIntent := strings.TrimSpace(body.TaskIntent)
	networkTier := body.NetworkTier
	codexSandbox := strings.TrimSpace(body.CodexSandbox)
	codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
	codexSearch := body.CodexSearch
	claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
	claudeSandbox := body.ClaudeSandbox
	claudeDomains := body.ClaudeWebFetchDomains

	if !explicitSafety {
		unsafe = src.UnsafeAutomation
		safetyPreset = strings.TrimSpace(src.SafetyPreset)
		taskIntent = strings.TrimSpace(src.TaskIntent)
		networkTier = src.NetworkTier
		codexSandbox = strings.TrimSpace(src.CodexSandbox)
		codexApprovalPolicy = strings.TrimSpace(src.CodexApprovalPolicy)
		codexSearch = src.CodexSearch
		claudePermissionMode = strings.TrimSpace(src.ClaudePermissionMode)
		claudeSandbox = src.ClaudeSandbox
		claudeDomains = append([]string{}, src.ClaudeWebFetchDomains...)
	}

	in := tasks.CreateTaskInput{
		WorkerType:            src.WorkerType,
		Mode:                  tasks.ModeNew,
		ConversationID:        src.ConversationID,
		UnsafeAutomation:      unsafe,
		SafetyEnvelope:        safetyEnvelope,
		SafetyPreset:          safetyPreset,
		TaskIntent:            taskIntent,
		NetworkTier:           networkTier,
		CodexSandbox:          codexSandbox,
		CodexApprovalPolicy:   codexApprovalPolicy,
		CodexSearch:           codexSearch,
		ClaudePermissionMode:  claudePermissionMode,
		ClaudeSandbox:         claudeSandbox,
		ClaudeWebFetchDomains: claudeDomains,
		Prompt:                ctxPrompt,
		WorkDir:               src.WorkDir,
		SessionID:             "",
	}

	envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
	in, ap := runsafe.ApplyAutopilot(ctx, in, runsafe.ApplyOptions{
		Driver:   driver,
		Envelope: envelope,
		Classify: runsafe.ClassifyOptions{},
	})

	newTask, err := s.Tasks.CreateTask(ctx, in)
	if err != nil {
		return tasks.Task{}, err
	}
	_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", src.ID, strings.TrimSpace(src.SessionID)))
	if ap.Applied {
		if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
			_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
		}
	}

	return s.startTask(ctx, newTask)
}

func (s *Service) EnterUnsafeTask(ctx context.Context, id string, prompt string) (tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return tasks.Task{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task id is required", "", nil, nil)
	}

	src, err := s.Tasks.GetTask(ctx, id)
	if err != nil {
		return tasks.Task{}, err
	}
	if strings.TrimSpace(src.ConversationID) == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "task has no conversation_id", "", nil, nil)
	}

	if s.Workers != nil {
		_, _ = s.Workers.Cancel(ctx, id)
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "continue"
	}

	mode := tasks.ModeNew
	sessionID := ""
	ctxPrompt := prompt
	if strings.TrimSpace(src.SessionID) != "" {
		mode = tasks.ModeResume
		sessionID = strings.TrimSpace(src.SessionID)
	} else {
		driver := src.WorkerType
		if s.Tools != nil {
			if profile, ok := s.Tools.Resolve(string(src.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		if driver == tasks.WorkerClaudeCode {
			rehydrated, err := tasks.BuildRehydratePrompt(ctx, s.Tasks, strings.TrimSpace(src.ConversationID), prompt)
			if err != nil {
				return tasks.Task{}, err
			}
			ctxPrompt = rehydrated
		}
	}

	in := tasks.CreateTaskInput{
		WorkerType:       src.WorkerType,
		Mode:             mode,
		ConversationID:   src.ConversationID,
		UnsafeAutomation: true,
		SafetyPreset:     "unsafe",
		TaskIntent:       "install",
		NetworkTier:      tasks.NetworkTierExecNet,
		Prompt:           ctxPrompt,
		WorkDir:          src.WorkDir,
		WorkDirStrategy:  "wait",
		SessionID:        sessionID,
		Warning:          src.Warning,
	}

	restoreCreated, restoreRef, err := s.ensureAutoRestorePoint(ctx, src)
	if err != nil {
		return tasks.Task{}, err
	}

	newTask, err := s.Tasks.CreateTask(ctx, in)
	if err != nil {
		return tasks.Task{}, err
	}
	_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("enter-unsafe: from run=%s", src.ID))
	if restoreCreated {
		_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("restore-point: %s", strings.TrimSpace(restoreRef)))
	}
	if newTask.Status != tasks.StatusQueued {
		if s.Hub != nil {
			s.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
		}
		return newTask, nil
	}
	return s.startTask(ctx, newTask)
}

func (s *Service) ensureAutoRestorePoint(ctx context.Context, src tasks.Task) (bool, string, error) {
	if s == nil || s.Tasks == nil {
		return false, "", errors.New("tasks store not configured")
	}
	actionType := "task.enter_unsafe"
	actionRef := strings.TrimSpace(src.ID)
	existing, err := s.Tasks.ListRollbackProofsByAction(ctx, src.ID, actionType, actionRef, tasks.ListRollbackProofsOptions{
		ProofType: "restore_point",
		Limit:     1,
	})
	if err != nil {
		return false, "", err
	}
	if len(existing) > 0 {
		return false, strings.TrimSpace(existing[0].ProofRef), nil
	}

	ref := fmt.Sprintf("auto:%d", time.Now().UTC().UnixMilli())
	detail, _ := json.Marshal(map[string]any{
		"task_id":  strings.TrimSpace(src.ID),
		"workdir":  strings.TrimSpace(src.WorkDir),
		"strategy": strings.TrimSpace(src.WorkDirStrategy),
		"source":   "enter_unsafe",
	})
	if _, err := s.Tasks.CreateRollbackProof(ctx, tasks.CreateRollbackProofInput{
		TaskID:     src.ID,
		ActionType: actionType,
		ActionRef:  actionRef,
		ProofType:  "restore_point",
		ProofRef:   ref,
		Detail:     detail,
	}); err != nil {
		return false, "", err
	}
	return true, ref, nil
}

func (s *Service) DecideApproval(ctx context.Context, taskID string, approvalID string, decision string, reason string) (tasks.ApprovalRequest, error) {
	if s == nil || s.Tasks == nil {
		return tasks.ApprovalRequest{}, errors.New("tasks store not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return tasks.ApprovalRequest{}, errors.New("task_id is required")
	}

	decision = strings.ToLower(strings.TrimSpace(decision))
	var status tasks.ApprovalStatus
	switch decision {
	case "approve":
		status = tasks.ApprovalStatusApproved
	case "deny":
		status = tasks.ApprovalStatusDenied
	default:
		return tasks.ApprovalRequest{}, errors.New("invalid decision")
	}

	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		list, err := s.Tasks.ListApprovalRequestsByTask(ctx, taskID, tasks.ListApprovalRequestsOptions{Status: tasks.ApprovalStatusPending, Limit: 1})
		if err != nil {
			return tasks.ApprovalRequest{}, err
		}
		if len(list) == 0 {
			return tasks.ApprovalRequest{}, errors.New("no pending approval found")
		}
		approvalID = strings.TrimSpace(list[0].ID)
	}

	ar, ok, err := s.Tasks.GetApprovalRequest(ctx, approvalID)
	if err != nil {
		return tasks.ApprovalRequest{}, err
	}
	if !ok || strings.TrimSpace(ar.TaskID) != taskID {
		return tasks.ApprovalRequest{}, errors.New("approval not found")
	}
	if ar.Status != tasks.ApprovalStatusPending {
		return tasks.ApprovalRequest{}, &tasks.ApprovalNotPendingError{
			ApprovalID: approvalID,
			Status:     ar.Status,
		}
	}

	reason = strings.TrimSpace(reason)
	forwarded := false
	if s.Workers != nil {
		if fw, ok := s.Workers.(approvalForwarder); ok && fw != nil {
			if err := fw.SubmitApprovalDecision(ctx, taskID, approvalID, decision, reason); err != nil {
				return tasks.ApprovalRequest{}, &RunnerUnavailableError{TaskID: taskID, Err: err}
			}
			forwarded = true
		}
	}

	if err := s.Tasks.UpdateApprovalRequestDecision(ctx, approvalID, tasks.UpdateApprovalRequestDecisionInput{
		Status: status,
		Reason: reason,
	}); err != nil {
		var notPending *tasks.ApprovalNotPendingError
		if !errors.As(err, &notPending) {
			return tasks.ApprovalRequest{}, err
		}
		if !forwarded {
			return tasks.ApprovalRequest{}, err
		}
		out, ok, getErr := s.Tasks.GetApprovalRequest(ctx, approvalID)
		if getErr != nil {
			return tasks.ApprovalRequest{}, getErr
		}
		if !ok {
			return tasks.ApprovalRequest{}, errors.New("approval not found")
		}
		if out.Status != status {
			return tasks.ApprovalRequest{}, err
		}
		return out, nil
	}

	out, ok, err := s.Tasks.GetApprovalRequest(ctx, approvalID)
	if err != nil {
		return tasks.ApprovalRequest{}, err
	}
	if !ok {
		return tasks.ApprovalRequest{}, errors.New("approval not found")
	}
	return out, nil
}

func (s *Service) ResolveSessionKeyByTaskID(ctx context.Context, taskID string) (string, tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return "", tasks.Task{}, errors.New("tasks store not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", tasks.Task{}, errors.New("task_id is required")
	}
	t, err := s.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", tasks.Task{}, err
	}
	return tasks.SessionKeyForTask(t), t, nil
}

func (s *Service) AppendActionAuditLog(ctx context.Context, taskID string, action string, payload any, err error) {
	if s == nil || s.Tasks == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = "secretary.action"
	}
	res := "ok"
	if err != nil {
		res = "error: " + strings.TrimSpace(err.Error())
	}
	pb, _ := json.Marshal(payload)
	line := fmt.Sprintf("secretary action: %s result=%s payload=%s", action, res, strings.TrimSpace(string(pb)))
	_, _ = s.Tasks.AppendLog(ctx, taskID, tasks.LogSystem, line)
}

func (s *Service) resolveContinueContext(ctx context.Context, key string) (string, []tasks.Task, tasks.Task, error) {
	conversationID, err := resolveConversationIDForSessionKey(ctx, s.Tasks, key)
	if err != nil {
		return "", nil, tasks.Task{}, err
	}
	runs, err := s.Tasks.ListTasksByConversationID(ctx, conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return "", nil, tasks.Task{}, err
	}
	if len(runs) == 0 {
		return "", nil, tasks.Task{}, newMutationError(404, MutationErrorNotFound, "session not found", "", nil, nil)
	}
	latest := runs[0]
	if latest.SessionDeletedAt != nil {
		return "", nil, tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "session is deleted; cannot continue", "", nil, nil)
	}
	return conversationID, runs, latest, nil
}

func isInFlightStatus(status tasks.Status) bool {
	return status == tasks.StatusRunning ||
		status == tasks.StatusQueued ||
		status == tasks.StatusWaiting ||
		status == tasks.StatusAwaitingApproval
}

func findInFlightRun(runs []tasks.Task) (tasks.Task, bool) {
	for _, t := range runs {
		if isInFlightStatus(t.Status) {
			return t, true
		}
	}
	return tasks.Task{}, false
}

func (s *Service) enqueueSessionContinue(
	ctx context.Context,
	conversationID string,
	body RunOptions,
	priority int,
	source string,
	inFlight *tasks.Task,
	preemptedTaskID string,
) (QueueAck, error) {
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = "continue"
		body.Prompt = prompt
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return QueueAck{}, err
	}
	item, err := s.Tasks.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         prompt,
		RunOptionsJSON: string(raw),
		Priority:       priority,
		Source:         source,
	})
	if err != nil {
		return QueueAck{}, err
	}
	pos, err := s.Tasks.SessionContinueQueuePosition(ctx, item.ID)
	if err != nil {
		return QueueAck{}, err
	}

	ack := QueueAck{Queued: true, QueueID: item.ID, Position: pos}
	if inFlight != nil {
		ack.ExistingTaskID = strings.TrimSpace(inFlight.ID)
		ack.ExistingStatus = string(inFlight.Status)
	}
	if strings.TrimSpace(preemptedTaskID) != "" {
		ack.PreemptedTaskID = strings.TrimSpace(preemptedTaskID)
	}
	return ack, nil
}

func (s *Service) createContinueTask(ctx context.Context, conversationID string, latest tasks.Task, body RunOptions) (tasks.Task, error) {
	if shouldContinueViaRehydrate(latest) {
		driver := latest.WorkerType
		if s.Tools != nil {
			if profile, ok := s.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		if driver != tasks.WorkerClaudeCode {
			return tasks.Task{}, errors.New("rehydrate is only supported for claude-code sessions")
		}

		prompt := strings.TrimSpace(body.Prompt)
		if prompt == "" {
			prompt = "continue"
		}
		ctxPrompt, err := tasks.BuildRehydratePrompt(ctx, s.Tasks, conversationID, prompt)
		if err != nil {
			return tasks.Task{}, err
		}

		explicitSafety := body.UnsafeAutomation ||
			strings.TrimSpace(body.SafetyPreset) != "" ||
			strings.TrimSpace(body.TaskIntent) != "" ||
			strings.TrimSpace(string(body.NetworkTier)) != "" ||
			strings.TrimSpace(body.CodexSandbox) != "" ||
			strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
			body.CodexSearch ||
			strings.TrimSpace(body.ClaudePermissionMode) != "" ||
			body.ClaudeSandbox ||
			len(body.ClaudeWebFetchDomains) > 0

		unsafe := body.UnsafeAutomation
		safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
		safetyPreset := strings.TrimSpace(body.SafetyPreset)
		taskIntent := strings.TrimSpace(body.TaskIntent)
		networkTier := body.NetworkTier
		codexSandbox := strings.TrimSpace(body.CodexSandbox)
		codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
		codexSearch := body.CodexSearch
		claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
		claudeSandbox := body.ClaudeSandbox
		claudeDomains := body.ClaudeWebFetchDomains

		if !explicitSafety {
			unsafe = latest.UnsafeAutomation
			safetyPreset = strings.TrimSpace(latest.SafetyPreset)
			taskIntent = strings.TrimSpace(latest.TaskIntent)
			networkTier = latest.NetworkTier
			codexSandbox = strings.TrimSpace(latest.CodexSandbox)
			codexApprovalPolicy = strings.TrimSpace(latest.CodexApprovalPolicy)
			codexSearch = latest.CodexSearch
			claudePermissionMode = strings.TrimSpace(latest.ClaudePermissionMode)
			claudeSandbox = latest.ClaudeSandbox
			claudeDomains = append([]string{}, latest.ClaudeWebFetchDomains...)
		}

		in := tasks.CreateTaskInput{
			WorkerType:            latest.WorkerType,
			Mode:                  tasks.ModeNew,
			ConversationID:        conversationID,
			UnsafeAutomation:      unsafe,
			SafetyEnvelope:        safetyEnvelope,
			SafetyPreset:          safetyPreset,
			TaskIntent:            taskIntent,
			NetworkTier:           networkTier,
			CodexSandbox:          codexSandbox,
			CodexApprovalPolicy:   codexApprovalPolicy,
			CodexSearch:           codexSearch,
			ClaudePermissionMode:  claudePermissionMode,
			ClaudeSandbox:         claudeSandbox,
			ClaudeWebFetchDomains: claudeDomains,
			Prompt:                ctxPrompt,
			WorkDir:               latest.WorkDir,
			SessionID:             "",
		}

		envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
		in, ap := runsafe.ApplyAutopilot(ctx, in, runsafe.ApplyOptions{
			Driver:   driver,
			Envelope: envelope,
			Classify: runsafe.ClassifyOptions{},
		})

		newTask, err := s.Tasks.CreateTask(ctx, in)
		if err != nil {
			return tasks.Task{}, err
		}
		_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", latest.ID, strings.TrimSpace(latest.SessionID)))

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
				_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
			}
		}
		return s.startTask(ctx, newTask)
	}

	if strings.TrimSpace(latest.SessionID) == "" {
		return tasks.Task{}, errors.New("task has no session_id to resume")
	}

	explicitSafety := body.UnsafeAutomation ||
		strings.TrimSpace(body.SafetyPreset) != "" ||
		strings.TrimSpace(body.TaskIntent) != "" ||
		strings.TrimSpace(string(body.NetworkTier)) != "" ||
		strings.TrimSpace(body.CodexSandbox) != "" ||
		strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
		body.CodexSearch ||
		strings.TrimSpace(body.ClaudePermissionMode) != "" ||
		body.ClaudeSandbox ||
		len(body.ClaudeWebFetchDomains) > 0

	unsafe := body.UnsafeAutomation
	safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
	safetyPreset := strings.TrimSpace(body.SafetyPreset)
	taskIntent := strings.TrimSpace(body.TaskIntent)
	networkTier := body.NetworkTier
	codexSandbox := strings.TrimSpace(body.CodexSandbox)
	codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
	codexSearch := body.CodexSearch
	claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
	claudeSandbox := body.ClaudeSandbox
	claudeDomains := body.ClaudeWebFetchDomains

	if !explicitSafety {
		unsafe = latest.UnsafeAutomation
		safetyPreset = strings.TrimSpace(latest.SafetyPreset)
		taskIntent = strings.TrimSpace(latest.TaskIntent)
		networkTier = latest.NetworkTier
		codexSandbox = strings.TrimSpace(latest.CodexSandbox)
		codexApprovalPolicy = strings.TrimSpace(latest.CodexApprovalPolicy)
		codexSearch = latest.CodexSearch
		claudePermissionMode = strings.TrimSpace(latest.ClaudePermissionMode)
		claudeSandbox = latest.ClaudeSandbox
		claudeDomains = append([]string{}, latest.ClaudeWebFetchDomains...)
	}

	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = "continue"
	}

	resumeIn := tasks.CreateTaskInput{
		WorkerType:            latest.WorkerType,
		Mode:                  tasks.ModeResume,
		ConversationID:        conversationID,
		UnsafeAutomation:      unsafe,
		SafetyEnvelope:        safetyEnvelope,
		SafetyPreset:          safetyPreset,
		TaskIntent:            taskIntent,
		NetworkTier:           networkTier,
		CodexSandbox:          codexSandbox,
		CodexApprovalPolicy:   codexApprovalPolicy,
		CodexSearch:           codexSearch,
		ClaudePermissionMode:  claudePermissionMode,
		ClaudeSandbox:         claudeSandbox,
		ClaudeWebFetchDomains: claudeDomains,
		Prompt:                prompt,
		WorkDir:               latest.WorkDir,
		SessionID:             strings.TrimSpace(latest.SessionID),
		Warning:               latest.Warning,
	}

	driver := latest.WorkerType
	if s.Tools != nil {
		if profile, ok := s.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
			driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
		}
	}
	envelope := runsafe.SafetyEnvelope(strings.TrimSpace(resumeIn.SafetyEnvelope))
	resumeIn, ap := runsafe.ApplyAutopilot(ctx, resumeIn, runsafe.ApplyOptions{
		Driver:   driver,
		Envelope: envelope,
		Classify: runsafe.ClassifyOptions{},
	})

	newTask, err := s.Tasks.CreateTask(ctx, resumeIn)
	if err != nil {
		return tasks.Task{}, err
	}
	if ap.Applied {
		if audit := runsafe.FormatAuditLog(driver, ap.Decision, resumeIn, true); strings.TrimSpace(audit) != "" {
			_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
		}
	}
	return s.startTask(ctx, newTask)
}

func (s *Service) CreateContinueTaskForConversation(ctx context.Context, conversationID string, body RunOptions) (tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return tasks.Task{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "conversation_id is required", "", nil, nil)
	}
	runs, err := s.Tasks.ListTasksByConversationID(ctx, conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return tasks.Task{}, err
	}
	if len(runs) == 0 {
		return tasks.Task{}, newMutationError(404, MutationErrorNotFound, "session not found", "", nil, nil)
	}
	latest := runs[0]
	if latest.SessionDeletedAt != nil {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "session is deleted; cannot continue", "", nil, nil)
	}
	if latest.Status == tasks.StatusBlocked {
		return tasks.Task{}, newMutationError(409, MutationErrorUnsupported, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", "", map[string]any{
			"existing_task_id": strings.TrimSpace(latest.ID),
			"existing_status":  strings.TrimSpace(string(latest.Status)),
		}, nil)
	}
	if inFlight, ok := findInFlightRun(runs); ok {
		return tasks.Task{}, newMutationError(409, MutationErrorSessionTaskInFlight, "session already has an in-flight task", "", map[string]any{
			"existing_task_id": strings.TrimSpace(inFlight.ID),
			"existing_status":  strings.TrimSpace(string(inFlight.Status)),
		}, nil)
	}
	return s.createContinueTask(ctx, conversationID, latest, body)
}

func (s *Service) startTask(ctx context.Context, newTask tasks.Task) (tasks.Task, error) {
	if s.Hub != nil {
		s.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
	}
	if s.Workers != nil {
		if err := s.Workers.Start(ctx, newTask.ID); err != nil {
			_, _ = s.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("runner start failed: %v", err))
			_ = s.Tasks.FinishTask(ctx, newTask.ID, tasks.FinishTaskInput{
				Status:     tasks.StatusFailed,
				ExitCode:   nil,
				Error:      err.Error(),
				SessionID:  strings.TrimSpace(newTask.SessionID),
				FinishedAt: time.Now().UTC(),
			})
			if s.Hub != nil {
				if updated, err2 := s.Tasks.GetTask(ctx, newTask.ID); err2 == nil {
					s.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: updated})
				}
			}
			return tasks.Task{}, &RunnerUnavailableError{TaskID: newTask.ID, Err: err}
		}
	}
	return newTask, nil
}

func resolveConversationIDForSessionKey(ctx context.Context, store *tasks.Store, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("session key is required")
	}
	if store == nil {
		return "", errors.New("tasks store not configured")
	}
	if strings.HasPrefix(key, "c:") {
		cid := strings.TrimSpace(strings.TrimPrefix(key, "c:"))
		if cid == "" {
			return "", errors.New("conversation_id is required")
		}
		return cid, nil
	}
	if strings.HasPrefix(key, "t:") {
		taskID := strings.TrimSpace(strings.TrimPrefix(key, "t:"))
		if taskID == "" {
			return "", errors.New("task_id is required")
		}
		t, err := store.GetTask(ctx, taskID)
		if err != nil {
			return "", fmt.Errorf("task not found: %w", err)
		}
		if cid := strings.TrimSpace(t.ConversationID); cid != "" {
			return cid, nil
		}
		return strings.TrimSpace(t.ID), nil
	}
	if strings.HasPrefix(key, "s:") {
		sid := strings.TrimSpace(strings.TrimPrefix(key, "s:"))
		if sid == "" {
			return "", errors.New("session_id is required")
		}
		if cid, ok, err := store.ConversationIDForSessionID(ctx, sid); err != nil {
			return "", err
		} else if ok {
			return cid, nil
		}
		return "", errors.New("session not found")
	}
	return "", errors.New("invalid session key (expected c:/s:/t:)")
}

func shouldContinueViaRehydrate(t tasks.Task) bool {
	if t.Mode != tasks.ModeResume {
		return false
	}
	if isNoConversationFound(t.Warning) {
		return true
	}
	if isNoConversationFound(t.Error) {
		return true
	}
	return false
}

func isNoConversationFound(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "no conversation found") {
		return false
	}
	return strings.Contains(lower, "session")
}
