package secretary

import (
	"controlccx/internal/agentsdk"
	sectools "controlccx/internal/secretary/tools"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

func newToolRegistry(store *tasks.Store) *agentsdk.ToolRegistry {
	return sectools.NewRegistry(sectools.Deps{Tasks: store})
}

func newToolRegistryWithOps(store *tasks.Store, ops *taskops.Service) *agentsdk.ToolRegistry {
	return sectools.NewRegistry(sectools.Deps{Tasks: store, Ops: ops})
}
