package tools

import (
	"context"

	"controlccx/internal/agentsdk"
	"controlccx/internal/systeminfo"
)

type systemInfoTool struct{}

func (systemInfoTool) Name() string { return "system_info" }

func (systemInfoTool) DescriptionZH() string {
	return "获取服务器系统信息快照（操作系统、架构、主机名等）。无参数。"
}

func (systemInfoTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	_ = call
	_ = deps
	return systeminfo.Snapshot(), nil
}
