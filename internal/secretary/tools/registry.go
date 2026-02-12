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
		out = append(out, Descriptor{Name: name, DescriptionZH: strings.TrimSpace(t.DescriptionZH())})
	}
	return out
}

func NewRegistry(deps Deps) *agentsdk.ToolRegistry {
	reg := agentsdk.NewToolRegistry()
	for _, t := range DefaultTools() {
		tool := t
		_ = reg.Register(tool.Name(), func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
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
