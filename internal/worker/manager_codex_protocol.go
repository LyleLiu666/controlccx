package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/tasks"
)

func normalizeCodexApprovalPolicy(raw string, unsafe bool) string {
	if unsafe {
		return "never"
	}
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "untrusted", "on-failure", "on-request", "never":
		return v
	case "":
		// Safe default: require approvals for non-trusted actions.
		return "untrusted"
	default:
		return "untrusted"
	}
}

func normalizeCodexSandbox(raw string, unsafe bool) string {
	if unsafe {
		return "danger-full-access"
	}
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "read-only", "workspace-write", "danger-full-access":
		return v
	case "":
		return "workspace-write"
	default:
		return "workspace-write"
	}
}

func (m *Manager) codexModelAndEffort() (string, string) {
	model := "gpt-5.2"
	effort := "xhigh"
	if m != nil && m.auth != nil {
		secrets := m.auth.Get()
		if strings.TrimSpace(secrets.CodexModel) != "" {
			model = strings.TrimSpace(secrets.CodexModel)
		}
		if strings.TrimSpace(secrets.CodexReasoningEffort) != "" {
			effort = strings.TrimSpace(secrets.CodexReasoningEffort)
		}
	}
	return model, effort
}

func (m *Manager) runCodexAppServer(ctx context.Context, task tasks.Task, peer *codexAppServerPeer, prompt string, resumeFailure *resumeFailureState, cancel context.CancelFunc) {
	if m == nil || peer == nil || m.store == nil {
		return
	}

	unsafe := task.UnsafeAutomation || m.cfg.Workers.UnsafeAutomation
	approvalPolicy := normalizeCodexApprovalPolicy(task.CodexApprovalPolicy, unsafe)
	sandbox := normalizeCodexSandbox(task.CodexSandbox, unsafe)
	model, effort := m.codexModelAndEffort()

	failOnce := func(err error, prefix string) {
		if err == nil {
			return
		}
		msg := strings.TrimSpace(err.Error())
		if prefix != "" {
			msg = strings.TrimSpace(prefix) + ": " + msg
		}
		if resumeFailure != nil {
			_ = resumeFailure.setOnce(msg)
		}
		if cancel != nil {
			cancel()
		}
		_ = peer.CloseStdin()
	}

	_, err := peer.Request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "controlccx",
			"title":   "controlccx",
			"version": "dev",
		},
	})
	if err != nil {
		failOnce(err, "codex initialize failed")
		return
	}
	_ = peer.Notify("initialized", nil)

	threadID := strings.TrimSpace(task.SessionID)
	if task.Mode == tasks.ModeResume && threadID != "" {
		raw, err := peer.Request(ctx, "thread/resume", map[string]any{
			"threadId":       threadID,
			"approvalPolicy": approvalPolicy,
			"sandbox":        sandbox,
		})
		if err != nil {
			failOnce(err, "codex thread/resume failed")
			return
		}
		if observed := strings.TrimSpace(extractCodexThreadID(raw)); observed != "" {
			threadID = observed
		}
	} else {
		raw, err := peer.Request(ctx, "thread/start", map[string]any{
			"approvalPolicy": approvalPolicy,
			"sandbox":        sandbox,
		})
		if err != nil {
			failOnce(err, "codex thread/start failed")
			return
		}
		threadID = strings.TrimSpace(extractCodexThreadID(raw))
	}
	if threadID != "" {
		_ = m.store.SetSessionID(context.Background(), task.ID, threadID)
		m.publishTaskUpdatedForce(task.ID)
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "continue"
	}
	turnParams := map[string]any{
		"threadId": threadID,
		"input": []any{
			map[string]any{
				"type": "text",
				"text": prompt,
			},
		},
	}
	// Best-effort per-turn overrides. Codex will ignore unknowns.
	if strings.TrimSpace(model) != "" {
		turnParams["model"] = strings.TrimSpace(model)
	}
	if strings.TrimSpace(effort) != "" {
		turnParams["effort"] = strings.TrimSpace(effort)
	}

	if _, err := peer.Request(ctx, "turn/start", turnParams); err != nil {
		failOnce(err, "codex turn/start failed")
		return
	}

	if err := peer.WaitDone(ctx); err != nil && !errors.Is(err, context.Canceled) {
		failOnce(err, "codex turn failed")
		return
	}
	_ = peer.CloseStdin()
}

func extractCodexThreadID(raw json.RawMessage) string {
	var resp struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ""
	}
	return strings.TrimSpace(resp.Thread.ID)
}

func codexApprovalRiskLevel(method string) tasks.RiskLevel {
	switch strings.TrimSpace(method) {
	case "item/fileChange/requestApproval", "applyPatchApproval":
		return tasks.RiskMedium
	case "item/commandExecution/requestApproval", "execCommandApproval":
		return tasks.RiskHigh
	default:
		return tasks.RiskHigh
	}
}

func codexApprovalSummary(method string, params json.RawMessage) string {
	method = strings.TrimSpace(method)
	switch method {
	case "item/commandExecution/requestApproval":
		var p struct {
			Command *string `json:"command"`
			Cwd     *string `json:"cwd"`
			Reason  *string `json:"reason"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			if p.Command != nil && strings.TrimSpace(*p.Command) != "" {
				return strings.TrimSpace(*p.Command)
			}
			if p.Reason != nil && strings.TrimSpace(*p.Reason) != "" {
				return strings.TrimSpace(*p.Reason)
			}
			if p.Cwd != nil && strings.TrimSpace(*p.Cwd) != "" {
				return "exec (cwd: " + strings.TrimSpace(*p.Cwd) + ")"
			}
		}
		return "exec"
	case "execCommandApproval":
		var p struct {
			Command []string `json:"command"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			cmd := strings.Join(p.Command, " ")
			if strings.TrimSpace(cmd) != "" {
				return strings.TrimSpace(cmd)
			}
		}
		return "exec"
	case "item/fileChange/requestApproval":
		var p struct {
			Reason *string `json:"reason"`
		}
		if err := json.Unmarshal(params, &p); err == nil {
			if p.Reason != nil && strings.TrimSpace(*p.Reason) != "" {
				return strings.TrimSpace(*p.Reason)
			}
		}
		return "apply patch"
	case "applyPatchApproval":
		return "apply patch"
	default:
		return method
	}
}

func (m *Manager) handleCodexServerRequest(ctx context.Context, task tasks.Task, peer *codexAppServerPeer, requestID json.RawMessage, method string, params json.RawMessage) {
	if peer == nil {
		return
	}
	if m == nil || m.store == nil {
		_ = peer.SendError(requestID, -32000, "worker not configured")
		return
	}

	method = strings.TrimSpace(method)
	if method == "" {
		_ = peer.SendError(requestID, -32601, "missing method")
		return
	}

	// Unsafe mode: auto-approve to avoid blocking.
	if task.UnsafeAutomation || m.cfg.Workers.UnsafeAutomation {
		_ = peer.SendResult(requestID, codexApprovalResponse(method, true))
		return
	}

	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "execCommandApproval", "applyPatchApproval":
		// Supported approval methods.
	default:
		if method == "item/tool/requestUserInput" {
			// Not supported in controlccx yet; respond with an empty payload to avoid deadlocks.
			_ = peer.SendResult(requestID, map[string]any{"answers": map[string]any{}})
			return
		}
		_ = peer.SendError(requestID, -32601, "unsupported server request: "+method)
		return
	}

	raw := map[string]any{
		"method": method,
		"params": json.RawMessage(params),
	}
	rawJSON, _ := json.Marshal(raw)

	ar, err := m.store.CreateApprovalRequest(context.Background(), tasks.CreateApprovalRequestInput{
		TaskID:     task.ID,
		WorkerType: tasks.WorkerCodex,
		WorkDir:    task.WorkDir,
		ActionType: method,
		RiskLevel:  codexApprovalRiskLevel(method),
		Summary:    codexApprovalSummary(method, params),
		Raw:        rawJSON,
	})
	if err != nil {
		_ = peer.SendError(requestID, -32000, err.Error())
		return
	}

	_ = m.store.SetAwaitingApproval(context.Background(), task.ID)
	m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("awaiting approval: %s", method))
	m.publishTaskUpdatedForce(task.ID)

	outcome := m.waitForApprovalDecision(ctx, task.ID, ar.ID)
	if outcome.TimedOut || outcome.Cancelled {
		_ = m.store.UpdateApprovalRequestDecision(context.Background(), ar.ID, tasks.UpdateApprovalRequestDecisionInput{
			Status: tasks.ApprovalStatusExpired,
			Reason: outcome.Reason,
		})
	}

	_ = m.store.SetRunningStatus(context.Background(), task.ID)
	m.publishTaskUpdatedForce(task.ID)

	m.appendLog(task.ID, tasks.LogSystem, fmt.Sprintf("approval decided: method=%s decision=%s", method, outcome.Decision))

	approved := outcome.Decision == "approve"
	_ = peer.SendResult(requestID, codexApprovalResponse(method, approved))
}

func codexApprovalResponse(method string, approved bool) any {
	method = strings.TrimSpace(method)
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision := "decline"
		if approved {
			decision = "accept"
		}
		return map[string]any{"decision": decision}
	case "execCommandApproval", "applyPatchApproval":
		decision := "denied"
		if approved {
			decision = "approved"
		}
		return map[string]any{"decision": decision}
	default:
		// Default to "decline/denied" for unknown methods.
		decision := "decline"
		if approved {
			decision = "accept"
		}
		return map[string]any{"decision": decision}
	}
}
