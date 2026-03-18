package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
)

func DefaultTools() []Tool {
	return []Tool{
		systemInfoTool{},
		fsRootsTool{},
		fsPWDTool{},
		fsEntriesTool{},
		fsReadTextTool{},
		skillsListTool{},
		tasksCountTool{},
		tasksListTool{},
		taskNewSubmitTool{},
		taskCancelSubmitTool{},
		taskContinueSubmitTool{},
		taskPreemptContinueSubmitTool{},
		taskResumeSubmitTool{},
		taskRehydrateSubmitTool{},
		missionContractUpsertTool{},
		projectAutonomyPolicyUpsertTool{},
		rollbackPlaybookGenerateTool{},
		executionPlanLoopSubmitTool{},
		taskApprovalDecideTool{},
		taskEnterUnsafeSubmitTool{},
		taskLogsTailTool{},
		taskLogGetTool{},
		schedulerCreateTool{},
		schedulerListTool{},
		schedulerCancelTool{},
	}
}

func Descriptors() []Descriptor {
	list := DefaultTools()
	out := make([]Descriptor, 0, len(list))
	for _, t := range list {
		name := strings.TrimSpace(t.Name())
		if name == "" {
			continue
		}
		d := Descriptor{Name: name, DescriptionZH: strings.TrimSpace(t.DescriptionZH())}
		if pd, ok := t.(ParamDescriber); ok {
			d.Params = trimStringList(pd.Params())
			d.Required = trimStringList(pd.Required())
			d.AnyOfRequired = trimStringGroups(pd.AnyOfRequired())
		}
		if rd, ok := t.(ReturnsDescriber); ok {
			d.ReturnsZH = strings.TrimSpace(rd.ReturnsZH())
		}
		out = append(out, d)
	}
	return out
}

func trimStringList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimStringGroups(in [][]string) [][]string {
	out := make([][]string, 0, len(in))
	for _, group := range in {
		trimmed := trimStringList(group)
		if len(trimmed) == 0 {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func NewRegistry(deps Deps) *agentsdk.ToolRegistry {
	return newRegistryWithTools(deps, DefaultTools())
}

func newRegistryWithTools(deps Deps, tools []Tool) *agentsdk.ToolRegistry {
	reg := agentsdk.NewToolRegistry()
	for _, t := range tools {
		tool := t
		_ = reg.Register(tool.Name(), func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
			if pd, ok := tool.(ParamDescriber); ok {
				if err := validateRequired(call.Fields, pd.Required(), pd.AnyOfRequired()); err != nil {
					appendValidationAuditLog(ctx, deps, call, err)
					return nil, err
				}
			}
			if err := enforceWriteActionPlanGuard(ctx, deps, call); err != nil {
				return nil, err
			}
			return tool.Execute(ctx, call, deps)
		})
	}
	reg.OnMissing = func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
		_ = ctx
		name := strings.TrimSpace(call.Name)
		if name == "" {
			return nil, agentsdk.ErrToolNotFound
		}
		return nil, fmt.Errorf("%w: %s", agentsdk.ErrToolNotFound, name)
	}
	return reg
}

func enforceWriteActionPlanGuard(ctx context.Context, deps Deps, call agentsdk.ToolCall) error {
	if !deps.WriteGuardEnabled {
		return nil
	}
	if !isWriteCapableTool(call.Name) {
		return nil
	}

	builder := deps.ActionPlanBuilder
	if builder == nil {
		builder = buildActionPlan
	}
	plan, err := builder(call)
	if err != nil {
		if deps.OnWriteGuardBlock != nil {
			deps.OnWriteGuardBlock(err)
		}
		return fmt.Errorf("action_plan generate failed: %w", err)
	}
	if deps.ActionPlanMainRecorder == nil {
		recErr := fmt.Errorf("action_plan audit recorder is required for write-capable tool %q", strings.TrimSpace(call.Name))
		if deps.OnWriteGuardBlock != nil {
			deps.OnWriteGuardBlock(recErr)
		}
		return recErr
	}
	if err := deps.ActionPlanMainRecorder.RecordActionPlan(ctx, plan); err != nil {
		if deps.OnWriteGuardBlock != nil {
			deps.OnWriteGuardBlock(err)
		}
		return fmt.Errorf("action_plan audit record failed: %w", err)
	}
	if deps.OnActionPlanEmitted != nil {
		deps.OnActionPlanEmitted(plan)
	}
	if deps.ActionPlanEventRecorder != nil {
		if err := deps.ActionPlanEventRecorder.RecordActionPlan(ctx, plan); err != nil && deps.OnWriteGuardSideEffectErr != nil {
			deps.OnWriteGuardSideEffectErr(err)
		}
	}
	return nil
}

func buildActionPlan(call agentsdk.ToolCall) (ActionPlan, error) {
	toolName := strings.TrimSpace(call.Name)
	if toolName == "" {
		return ActionPlan{}, fmt.Errorf("tool name is required")
	}
	id := strings.TrimSpace(call.ID)
	if id == "" {
		id = fmt.Sprintf("ap_%d", time.Now().UTC().UnixNano())
	}
	fields := copyStringMap(call.Fields)
	taskID := strings.TrimSpace(fields["task_id"])
	conversationID := strings.TrimSpace(fields["conversation_id"])

	return ActionPlan{
		ID:             id,
		ToolName:       toolName,
		TaskID:         taskID,
		ConversationID: conversationID,
		RiskLevel:      riskLevelForTool(toolName),
		Fields:         fields,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func isWriteCapableTool(toolName string) bool {
	_, ok := writeCapableTools[strings.TrimSpace(toolName)]
	return ok
}

func riskLevelForTool(toolName string) string {
	if _, ok := highRiskWriteTools[strings.TrimSpace(toolName)]; ok {
		return "high"
	}
	return "medium"
}

var writeCapableTools = map[string]struct{}{
	"task_new_submit":                {},
	"task_cancel_submit":             {},
	"task_continue_submit":           {},
	"task_preempt_continue_submit":   {},
	"task_resume_submit":             {},
	"task_rehydrate_submit":          {},
	"mission_contract_upsert":        {},
	"project_autonomy_policy_upsert": {},
	"execution_plan_loop_submit":     {},
	"task_approval_decide":           {},
	"task_enter_unsafe_submit":       {},
	"scheduler_create":               {},
	"scheduler_cancel":               {},
}

var highRiskWriteTools = map[string]struct{}{
	"task_enter_unsafe_submit": {},
	"task_approval_decide":     {},
	"task_cancel_submit":       {},
}

func appendValidationAuditLog(ctx context.Context, deps Deps, call agentsdk.ToolCall, err error) {
	if err == nil || deps.Ops == nil || call.Fields == nil {
		return
	}
	taskID := strings.TrimSpace(call.Fields["task_id"])
	if taskID == "" {
		return
	}
	action := strings.TrimSpace(call.Name)
	if action == "" {
		action = "secretary.tool"
	}
	deps.Ops.AppendActionAuditLog(ctx, taskID, action, map[string]any{
		"task_id": taskID,
		"fields":  copyStringMap(call.Fields),
	}, err)
}
