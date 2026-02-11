package tools

import (
	"context"

	"controlccx/internal/agentsdk"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

type Tool interface {
	Name() string
	DescriptionZH() string
	Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error)
}

type Deps struct {
	Tasks *tasks.Store
	Ops   *taskops.Service
}

type Descriptor struct {
	Name          string
	DescriptionZH string
}
