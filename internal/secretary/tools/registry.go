package tools

import (
	"context"
	"fmt"
	"strings"

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
	reg := agentsdk.NewToolRegistry()
	for _, t := range DefaultTools() {
		tool := t
		_ = reg.Register(tool.Name(), func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
			if pd, ok := tool.(ParamDescriber); ok {
				if err := validateRequired(call.Fields, pd.Required(), pd.AnyOfRequired()); err != nil {
					appendValidationAuditLog(ctx, deps, call, err)
					return nil, err
				}
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
