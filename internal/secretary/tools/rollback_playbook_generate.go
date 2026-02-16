package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/tasks"
)

type rollbackPlaybookGenerateTool struct{}

func (rollbackPlaybookGenerateTool) Name() string { return "rollback_playbook_generate" }

func (rollbackPlaybookGenerateTool) DescriptionZH() string {
	return "根据已记录的 rollback proofs 生成分步回滚方案。参数：task_id（必填）；action_type + action_ref（可选，需同时提供，用于限定动作范围）。"
}

func (rollbackPlaybookGenerateTool) Params() []string {
	return []string{"task_id", "action_type", "action_ref"}
}

func (rollbackPlaybookGenerateTool) Required() []string { return []string{"task_id"} }

func (rollbackPlaybookGenerateTool) AnyOfRequired() [][]string { return nil }

type rollbackPlaybookStep struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	ProofID   string `json:"proof_id"`
	ProofType string `json:"proof_type"`
	ProofRef  string `json:"proof_ref"`
}

func (rollbackPlaybookGenerateTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	if deps.Tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	if _, err := deps.Tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}

	actionType := strings.TrimSpace(call.Fields["action_type"])
	actionRef := strings.TrimSpace(call.Fields["action_ref"])
	if (actionType == "") != (actionRef == "") {
		return nil, errors.New("action_type and action_ref must be provided together")
	}

	var (
		proofs []tasks.RollbackProof
		err    error
	)
	if actionType != "" {
		proofs, err = deps.Tasks.ListRollbackProofsByAction(ctx, taskID, actionType, actionRef, tasks.ListRollbackProofsOptions{Limit: 20})
	} else {
		proofs, err = deps.Tasks.ListRollbackProofsByTask(ctx, taskID, tasks.ListRollbackProofsOptions{Limit: 20})
	}
	if err != nil {
		return nil, err
	}
	if len(proofs) == 0 {
		return nil, errors.New("no rollback proofs found for task")
	}

	steps := make([]rollbackPlaybookStep, 0, len(proofs)+2)
	lines := make([]string, 0, len(proofs)+3)
	lines = append(lines, "回滚剧本（v1）")
	lines = append(lines, fmt.Sprintf("1. 锁定当前现场并记录 task_id=%s。", taskID))
	stepIdx := 2
	for _, proof := range proofs {
		title := fmt.Sprintf("使用 %s 还原到 %s。", defaultRollbackProofType(proof.ProofType), strings.TrimSpace(proof.ProofRef))
		steps = append(steps, rollbackPlaybookStep{
			Index:     stepIdx,
			Title:     title,
			ProofID:   strings.TrimSpace(proof.ID),
			ProofType: strings.TrimSpace(proof.ProofType),
			ProofRef:  strings.TrimSpace(proof.ProofRef),
		})
		lines = append(lines, fmt.Sprintf("%d. %s（proof=%s）", stepIdx, title, strings.TrimSpace(proof.ProofRef)))
		stepIdx++
	}
	lines = append(lines, fmt.Sprintf("%d. 运行回归验证并记录结果。", stepIdx))

	return map[string]any{
		"task_id":     taskID,
		"action_type": actionType,
		"action_ref":  actionRef,
		"steps":       steps,
		"playbook":    strings.Join(lines, "\n"),
	}, nil
}

func defaultRollbackProofType(proofType string) string {
	proofType = strings.TrimSpace(proofType)
	if proofType == "" {
		return "proof"
	}
	return proofType
}
