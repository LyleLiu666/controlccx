package tools

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

type taskApprovalDecideTool struct{}

func (taskApprovalDecideTool) Name() string { return "task_approval_decide" }

func (taskApprovalDecideTool) DescriptionZH() string {
	return "提交审批决策。参数：task_id（必填）、decision（必填：approve/deny）、approval_id（可选，缺失时自动定位最新pending）、reason（可选）。"
}

func (taskApprovalDecideTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	ops, err := requireOps(deps)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errorsNewTaskIDRequired()
	}
	decision := strings.ToLower(strings.TrimSpace(call.Fields["decision"]))
	approvalID := strings.TrimSpace(call.Fields["approval_id"])
	reason := strings.TrimSpace(call.Fields["reason"])
	ar, err := ops.DecideApproval(ctx, taskID, approvalID, decision, reason)
	ops.AppendActionAuditLog(ctx, taskID, "task_approval_decide", map[string]any{"task_id": taskID, "approval_id": approvalID, "decision": decision}, err)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":          true,
		"approval_id": ar.ID,
		"status":      ar.Status,
		"task_id":     ar.TaskID,
	}, nil
}
