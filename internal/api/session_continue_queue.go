package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"controlccx/internal/events"
	"controlccx/internal/runsafe"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

const staleWatchdogTimeout = 15 * time.Minute

type sessionContinueOptions struct {
	Prompt                string   `json:"prompt"`
	UnsafeAutomation      bool     `json:"unsafe_automation,omitempty"`
	SafetyEnvelope        string   `json:"safety_envelope,omitempty"`
	SafetyPreset          string   `json:"safety_preset,omitempty"`
	TaskIntent            string   `json:"task_intent,omitempty"`
	CodexSandbox          string   `json:"codex_sandbox,omitempty"`
	CodexApprovalPolicy   string   `json:"codex_approval_policy,omitempty"`
	CodexSearch           bool     `json:"codex_search,omitempty"`
	ClaudePermissionMode  string   `json:"claude_permission_mode,omitempty"`
	ClaudeSandbox         bool     `json:"claude_sandbox,omitempty"`
	ClaudeWebFetchDomains []string `json:"claude_webfetch_domains,omitempty"`
}

type runnerUnavailableError struct {
	taskID string
	err    error
}

func (e *runnerUnavailableError) Error() string {
	if e == nil || e.err == nil {
		return "runner unavailable"
	}
	return e.err.Error()
}

func (a *API) handleSessionContinue(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := decodeSessionContinueOptions(r.Body)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if a.TaskOps != nil {
		out, err := a.TaskOps.ContinueSession(r.Context(), key, toTaskOpsRunOptions(body))
		if err != nil {
			writeSessionContinueContextError(w, err)
			return
		}
		if out.Queue != nil {
			writeJSONStatus(w, http.StatusAccepted, out.Queue)
			return
		}
		if out.Task != nil {
			writeJSON(w, out.Task)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	conversationID, runs, latest, err := a.resolveContinueContext(r.Context(), key)
	if err != nil {
		writeSessionContinueContextError(w, err)
		return
	}
	if latest.Status == tasks.StatusBlocked {
		http.Error(w, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", http.StatusConflict)
		return
	}

	if inFlight, ok := findInFlightRun(runs); ok {
		ack, err := a.enqueueSessionContinue(r.Context(), conversationID, body, 0, "continue", &inFlight, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSONStatus(w, http.StatusAccepted, ack)
		return
	}

	newTask, err := a.createContinueTask(r.Context(), conversationID, latest, body)
	if err != nil {
		writeContinueTaskError(w, err)
		return
	}
	writeJSON(w, newTask)
}

func (a *API) handleSessionPreemptContinue(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := decodeSessionContinueOptions(r.Body)
	if err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if a.TaskOps != nil {
		ack, err := a.TaskOps.PreemptContinueSession(r.Context(), key, toTaskOpsRunOptions(body))
		if err != nil {
			var runnerErr *taskops.RunnerUnavailableError
			if errors.As(err, &runnerErr) {
				writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
					"error":   "runner_unavailable",
					"message": err.Error(),
					"hint":    "restart the runner daemon (controlccx-runnerd)",
					"task_id": strings.TrimSpace(runnerErr.TaskID),
				})
				return
			}
			writeSessionContinueContextError(w, err)
			return
		}
		writeJSONStatus(w, http.StatusAccepted, ack)
		go func() {
			_ = a.drainContinueQueuesOnce(context.Background())
		}()
		return
	}

	conversationID, runs, latest, err := a.resolveContinueContext(r.Context(), key)
	if err != nil {
		writeSessionContinueContextError(w, err)
		return
	}
	if latest.Status == tasks.StatusBlocked {
		http.Error(w, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", http.StatusConflict)
		return
	}

	preemptedTaskID := ""
	if inFlight, ok := findInFlightRun(runs); ok {
		preemptedTaskID = strings.TrimSpace(inFlight.ID)
		if a.Workers != nil {
			okCancel, err := a.Workers.Cancel(r.Context(), inFlight.ID)
			if err != nil {
				writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
					"error":   "runner_unavailable",
					"message": err.Error(),
					"hint":    "restart the runner daemon (controlccx-runnerd)",
					"task_id": inFlight.ID,
				})
				return
			}
			if !okCancel && (inFlight.Status == tasks.StatusQueued || inFlight.Status == tasks.StatusWaiting || inFlight.Status == tasks.StatusAwaitingApproval) {
				_ = a.Tasks.SetCanceled(r.Context(), inFlight.ID)
				if a.Hub != nil {
					if updated, err := a.Tasks.GetTask(r.Context(), inFlight.ID); err == nil {
						a.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: updated})
					}
				}
			}
		}
	}

	ack, err := a.enqueueSessionContinue(r.Context(), conversationID, body, 100, "preempt", nil, preemptedTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, http.StatusAccepted, ack)

	go func() {
		_ = a.drainContinueQueuesOnce(context.Background())
	}()
}

func (a *API) handleSessionContinueQueue(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	if a.TaskOps != nil {
		items, err := a.TaskOps.SessionContinueQueue(r.Context(), key, 200)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"items": items})
		return
	}
	conversationID, err := resolveConversationIDForSessionKey(r.Context(), a.Tasks, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items, err := a.Tasks.ListSessionContinueQueueByConversation(r.Context(), conversationID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func decodeSessionContinueOptions(body io.Reader) (sessionContinueOptions, error) {
	var out sessionContinueOptions
	if err := json.NewDecoder(body).Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return sessionContinueOptions{}, err
	}
	return out, nil
}

func writeSessionContinueContextError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch msg {
	case "session not found":
		http.Error(w, msg, http.StatusNotFound)
		return
	case "session is deleted; cannot continue":
		http.Error(w, msg, http.StatusBadRequest)
		return
	default:
		if strings.Contains(msg, "invalid session key") || strings.Contains(msg, "required") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func writeContinueTaskError(w http.ResponseWriter, err error) {
	var busy *tasks.WorkDirBusyError
	if errors.As(err, &busy) {
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"error":            "workdir_busy",
			"message":          err.Error(),
			"workdir":          strings.TrimSpace(busy.WorkDir),
			"existing_task_id": strings.TrimSpace(busy.ExistingTaskID),
			"existing_status":  busy.ExistingStatus,
		})
		return
	}
	var runnerErr *runnerUnavailableError
	if errors.As(err, &runnerErr) {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "runner_unavailable",
			"message": err.Error(),
			"hint":    "restart the runner daemon (controlccx-runnerd)",
			"task_id": strings.TrimSpace(runnerErr.taskID),
		})
		return
	}
	var runnerErr2 *taskops.RunnerUnavailableError
	if errors.As(err, &runnerErr2) {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "runner_unavailable",
			"message": err.Error(),
			"hint":    "restart the runner daemon (controlccx-runnerd)",
			"task_id": strings.TrimSpace(runnerErr2.TaskID),
		})
		return
	}
	msg := strings.TrimSpace(err.Error())
	if strings.Contains(msg, "session_id") || strings.Contains(msg, "rehydrate") || strings.Contains(msg, "session") {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	http.Error(w, msg, http.StatusInternalServerError)
}

func toTaskOpsRunOptions(in sessionContinueOptions) taskops.RunOptions {
	return taskops.RunOptions{
		Prompt:                in.Prompt,
		UnsafeAutomation:      in.UnsafeAutomation,
		SafetyEnvelope:        in.SafetyEnvelope,
		SafetyPreset:          in.SafetyPreset,
		TaskIntent:            in.TaskIntent,
		CodexSandbox:          in.CodexSandbox,
		CodexApprovalPolicy:   in.CodexApprovalPolicy,
		CodexSearch:           in.CodexSearch,
		ClaudePermissionMode:  in.ClaudePermissionMode,
		ClaudeSandbox:         in.ClaudeSandbox,
		ClaudeWebFetchDomains: in.ClaudeWebFetchDomains,
	}
}

func (a *API) resolveContinueContext(ctx context.Context, key string) (string, []tasks.Task, tasks.Task, error) {
	conversationID, err := resolveConversationIDForSessionKey(ctx, a.Tasks, key)
	if err != nil {
		return "", nil, tasks.Task{}, err
	}
	runs, err := a.Tasks.ListTasksByConversationID(ctx, conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return "", nil, tasks.Task{}, err
	}
	if len(runs) == 0 {
		return "", nil, tasks.Task{}, errors.New("session not found")
	}
	latest := runs[0]
	if latest.SessionDeletedAt != nil {
		return "", nil, tasks.Task{}, errors.New("session is deleted; cannot continue")
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

func (a *API) enqueueSessionContinue(
	ctx context.Context,
	conversationID string,
	body sessionContinueOptions,
	priority int,
	source string,
	inFlight *tasks.Task,
	preemptedTaskID string,
) (map[string]any, error) {
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = "continue"
		body.Prompt = prompt
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	item, err := a.Tasks.EnqueueSessionContinue(ctx, tasks.EnqueueSessionContinueInput{
		ConversationID: conversationID,
		Prompt:         prompt,
		RunOptionsJSON: string(raw),
		Priority:       priority,
		Source:         source,
	})
	if err != nil {
		return nil, err
	}
	pos, err := a.Tasks.SessionContinueQueuePosition(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"queued":   true,
		"queue_id": item.ID,
		"position": pos,
	}
	if inFlight != nil {
		out["existing_task_id"] = strings.TrimSpace(inFlight.ID)
		out["existing_status"] = string(inFlight.Status)
	}
	if strings.TrimSpace(preemptedTaskID) != "" {
		out["preempted_task_id"] = strings.TrimSpace(preemptedTaskID)
	}
	return out, nil
}

func (a *API) createContinueTask(ctx context.Context, conversationID string, latest tasks.Task, body sessionContinueOptions) (tasks.Task, error) {
	if shouldContinueViaRehydrate(latest) {
		driver := latest.WorkerType
		if a.Tools != nil {
			if profile, ok := a.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
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
		ctxPrompt, err := tasks.BuildRehydratePrompt(ctx, a.Tasks, conversationID, prompt)
		if err != nil {
			return tasks.Task{}, err
		}

		explicitSafety := body.UnsafeAutomation ||
			strings.TrimSpace(body.SafetyPreset) != "" ||
			strings.TrimSpace(body.TaskIntent) != "" ||
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

		newTask, err := a.Tasks.CreateTask(ctx, in)
		if err != nil {
			return tasks.Task{}, err
		}
		_, _ = a.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", latest.ID, strings.TrimSpace(latest.SessionID)))

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
				_, _ = a.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
			}
		}
		return a.startContinueTask(ctx, newTask)
	}

	if strings.TrimSpace(latest.SessionID) == "" {
		return tasks.Task{}, errors.New("task has no session_id to resume")
	}

	explicitSafety := body.UnsafeAutomation ||
		strings.TrimSpace(body.SafetyPreset) != "" ||
		strings.TrimSpace(body.TaskIntent) != "" ||
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
	if a.Tools != nil {
		if profile, ok := a.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
			driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
		}
	}
	envelope := runsafe.SafetyEnvelope(strings.TrimSpace(resumeIn.SafetyEnvelope))
	resumeIn, ap := runsafe.ApplyAutopilot(ctx, resumeIn, runsafe.ApplyOptions{
		Driver:   driver,
		Envelope: envelope,
		Classify: runsafe.ClassifyOptions{},
	})

	newTask, err := a.Tasks.CreateTask(ctx, resumeIn)
	if err != nil {
		return tasks.Task{}, err
	}
	if ap.Applied {
		if audit := runsafe.FormatAuditLog(driver, ap.Decision, resumeIn, true); strings.TrimSpace(audit) != "" {
			_, _ = a.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, audit)
		}
	}
	return a.startContinueTask(ctx, newTask)
}

func (a *API) startContinueTask(ctx context.Context, newTask tasks.Task) (tasks.Task, error) {
	if a.Hub != nil {
		a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
	}
	if a.Workers != nil {
		if err := a.Workers.Start(ctx, newTask.ID); err != nil {
			_, _ = a.Tasks.AppendLog(ctx, newTask.ID, tasks.LogSystem, fmt.Sprintf("runner start failed: %v", err))
			_ = a.Tasks.FinishTask(ctx, newTask.ID, tasks.FinishTaskInput{
				Status:     tasks.StatusFailed,
				ExitCode:   nil,
				Error:      err.Error(),
				SessionID:  strings.TrimSpace(newTask.SessionID),
				FinishedAt: time.Now().UTC(),
			})
			if a.Hub != nil {
				if updated, err2 := a.Tasks.GetTask(ctx, newTask.ID); err2 == nil {
					a.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: updated})
				}
			}
			return tasks.Task{}, &runnerUnavailableError{taskID: newTask.ID, err: err}
		}
	}
	return newTask, nil
}

func (a *API) drainContinueQueuesOnce(ctx context.Context) error {
	if a == nil || a.Tasks == nil {
		return nil
	}
	conversations, err := a.Tasks.ListSessionContinueQueueConversations(ctx, 200)
	if err != nil {
		return err
	}
	for _, cid := range conversations {
		if err := a.drainContinueQueueConversation(ctx, cid); err != nil {
			return err
		}
	}
	return nil
}

func (a *API) drainContinueQueueConversation(ctx context.Context, conversationID string) error {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	runs, err := a.Tasks.ListTasksByConversationID(ctx, conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return nil
	}
	latest := runs[0]
	if latest.SessionDeletedAt != nil || latest.Status == tasks.StatusBlocked {
		return nil
	}
	if _, ok := findInFlightRun(runs); ok {
		return nil
	}

	item, ok, err := a.Tasks.ClaimNextSessionContinue(ctx, conversationID)
	if err != nil || !ok {
		return err
	}

	var body sessionContinueOptions
	if raw := strings.TrimSpace(item.RunOptionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStateFailed)
			return nil
		}
	}
	if strings.TrimSpace(body.Prompt) == "" {
		body.Prompt = strings.TrimSpace(item.Prompt)
	}
	if strings.TrimSpace(body.Prompt) == "" {
		_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStateFailed)
		return nil
	}

	newTask, err := a.createContinueTask(ctx, conversationID, latest, body)
	if err != nil {
		var busy *tasks.WorkDirBusyError
		var runnerErr *runnerUnavailableError
		switch {
		case errors.As(err, &busy), errors.As(err, &runnerErr):
			_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStatePending)
		default:
			_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStateFailed)
		}
		return nil
	}

	_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStateDone)
	if a.Hub != nil {
		a.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: newTask})
	}
	return nil
}

func (a *API) runStaleWatchdogOnce(ctx context.Context, now time.Time) (int, error) {
	if a == nil || a.Tasks == nil {
		return 0, nil
	}
	staleBefore := now.Add(-staleWatchdogTimeout)
	list, err := a.Tasks.ListStaleInFlightTasks(ctx, staleBefore, 200)
	if err != nil {
		return 0, err
	}
	if len(list) == 0 {
		return 0, nil
	}

	updated := 0
	for _, t := range list {
		ok, err := a.Tasks.InterruptTaskIfStaleInFlight(ctx, t.ID, staleBefore, now, "stale watchdog timeout")
		if err != nil {
			return updated, err
		}
		if !ok {
			continue
		}

		cancelOK := false
		cancelErr := error(nil)
		if a.Workers != nil {
			cancelCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			cancelOK, cancelErr = a.Workers.Cancel(cancelCtx, t.ID)
			cancel()
		}

		msg := fmt.Sprintf("stale watchdog: no heartbeat/log for %s; marking interrupted", staleWatchdogTimeout.String())
		if cancelErr != nil {
			msg = fmt.Sprintf("%s (cancel error: %v)", msg, cancelErr)
		} else if cancelOK {
			msg = msg + " (cancel requested)"
		}
		_, _ = a.Tasks.AppendLog(ctx, t.ID, tasks.LogSystem, msg)

		updated++
		if a.Hub != nil {
			if nt, err := a.Tasks.GetTask(ctx, t.ID); err == nil {
				a.Hub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: nt})
			}
		}
	}
	return updated, nil
}

func (a *API) StartBackgroundLoops(ctx context.Context) func() {
	if a == nil || a.Tasks == nil {
		return func() {}
	}
	_, _ = a.Tasks.ResetDispatchingSessionContinueToPending(ctx)
	stopCh := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(stopCh)
		})
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				_ = a.drainContinueQueuesOnce(context.Background())
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				_, _ = a.runStaleWatchdogOnce(context.Background(), time.Now().UTC())
			}
		}
	}()

	return stop
}
