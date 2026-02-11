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
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

const staleWatchdogTimeout = 15 * time.Minute

type sessionContinueOptions struct {
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
		writeTaskMutationInvalidJSON(w)
		return
	}
	ops := a.taskOpsOrShim()
	if ops == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	out, err := ops.ContinueSession(r.Context(), key, toTaskOpsRunOptions(body))
	if err != nil {
		writeTaskMutationProblem(w, err)
		return
	}
	if out.Queue != nil {
		writeTaskMutationResult(w, http.StatusAccepted, taskops.NewQueueMutationResult(taskops.ActionSessionContinue, *out.Queue))
		return
	}
	if out.Task != nil {
		writeTaskMutationResult(w, http.StatusOK, taskops.NewTaskMutationResult(taskops.ActionSessionContinue, *out.Task))
		return
	}
	writeTaskMutationResult(w, http.StatusOK, taskops.MutationResult{
		OK:     true,
		Action: taskops.ActionSessionContinue,
		Meta:   map[string]any{"ok": true},
	})
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
		writeTaskMutationInvalidJSON(w)
		return
	}
	ops := a.taskOpsOrShim()
	if ops == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	ack, err := ops.PreemptContinueSession(r.Context(), key, toTaskOpsRunOptions(body))
	if err != nil {
		writeTaskMutationProblem(w, err)
		return
	}
	writeTaskMutationResult(w, http.StatusAccepted, taskops.NewQueueMutationResult(taskops.ActionSessionPreemptContinue, ack))

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

func toTaskOpsRunOptions(in sessionContinueOptions) taskops.RunOptions {
	return taskops.RunOptions{
		Prompt:                in.Prompt,
		UnsafeAutomation:      in.UnsafeAutomation,
		SafetyEnvelope:        in.SafetyEnvelope,
		SafetyPreset:          in.SafetyPreset,
		TaskIntent:            in.TaskIntent,
		NetworkTier:           in.NetworkTier,
		CodexSandbox:          in.CodexSandbox,
		CodexApprovalPolicy:   in.CodexApprovalPolicy,
		CodexSearch:           in.CodexSearch,
		ClaudePermissionMode:  in.ClaudePermissionMode,
		ClaudeSandbox:         in.ClaudeSandbox,
		ClaudeWebFetchDomains: in.ClaudeWebFetchDomains,
	}
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

	ops := a.taskOpsOrShim()
	if ops == nil {
		_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStateFailed)
		return nil
	}
	newTask, err := ops.CreateContinueTaskForConversation(ctx, conversationID, toTaskOpsRunOptions(body))
	if err != nil {
		problem := taskops.ParseMutationError(err)
		keepPending := false
		switch problem.Error {
		case taskops.MutationErrorWorkdirBusy, taskops.MutationErrorRunnerUnavailable, taskops.MutationErrorSessionTaskInFlight:
			keepPending = true
		case taskops.MutationErrorUnsupported:
			status := strings.TrimSpace(fmt.Sprint(problem.Details["existing_status"]))
			keepPending = status == string(tasks.StatusBlocked)
		}
		if keepPending {
			_ = a.Tasks.MarkSessionContinueQueueState(ctx, item.ID, tasks.SessionContinueQueueStatePending)
		} else {
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
