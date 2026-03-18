package secretary

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/secretary/tools"
	"controlccx/internal/tasks"
)

type actionPlanMainRecorder struct {
	svc   *Service
	runID string
}

type actionPlanEventRecorder struct {
	svc   *Service
	runID string
}

func newActionPlanMainRecorder(svc *Service, runID string) tools.ActionPlanRecorder {
	return &actionPlanMainRecorder{svc: svc, runID: strings.TrimSpace(runID)}
}

func newActionPlanEventRecorder(svc *Service, runID string) tools.ActionPlanRecorder {
	return &actionPlanEventRecorder{svc: svc, runID: strings.TrimSpace(runID)}
}

func (r *actionPlanMainRecorder) RecordActionPlan(ctx context.Context, plan tools.ActionPlan) error {
	if r == nil || r.svc == nil {
		return fmt.Errorf("action_plan main recorder is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	taskID := strings.TrimSpace(plan.TaskID)
	if taskID != "" && r.svc.tasks != nil {
		pb, _ := json.Marshal(map[string]any{
			"action_plan_id":  plan.ID,
			"tool_name":       plan.ToolName,
			"risk_level":      plan.RiskLevel,
			"conversation_id": strings.TrimSpace(plan.ConversationID),
			"fields":          plan.Fields,
		})
		line := fmt.Sprintf("secretary action_plan: %s", strings.TrimSpace(string(pb)))
		if _, err := r.svc.tasks.AppendLog(ctx, taskID, tasks.LogSystem, line); err != nil {
			return err
		}
		return nil
	}

	if r.svc.events == nil {
		return fmt.Errorf("action_plan audit requires task_id or secretary event store")
	}
	runID := r.runID
	if runID == "" {
		runID = "action_plan:" + strings.TrimSpace(plan.ID)
	}
	return r.svc.events.Append(ctx, runID, agentsdk.Event{
		Kind:     agentsdk.EventKindTrace,
		Protocol: "action_plan",
		Step:     0,
		Time:     time.Now().UTC(),
		Payload: map[string]any{
			"kind":        "action_plan_audit",
			"action_plan": plan,
		},
	})
}

func (r *actionPlanEventRecorder) RecordActionPlan(ctx context.Context, plan tools.ActionPlan) error {
	if r == nil || r.svc == nil || r.svc.events == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runID := strings.TrimSpace(r.runID)
	if runID == "" {
		runID = "action_plan:" + strings.TrimSpace(plan.ID)
	}
	return r.svc.events.Append(ctx, runID, agentsdk.Event{
		Kind:     agentsdk.EventKindTrace,
		Protocol: "action_plan_side",
		Step:     0,
		Time:     time.Now().UTC(),
		Payload: map[string]any{
			"kind":        "action_plan_side",
			"action_plan": plan,
		},
	})
}
