package tools

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
)

func requireOps(deps Deps) (*taskops.Service, error) {
	if deps.Ops == nil {
		return nil, errors.New("task ops not configured")
	}
	return deps.Ops, nil
}

func resolveSessionByTaskID(ctx context.Context, ops *taskops.Service, fields map[string]string) (taskID string, key string, t tasks.Task, err error) {
	taskID = strings.TrimSpace(fields["task_id"])
	if taskID == "" {
		err = errors.New("task_id is required")
		return
	}
	key, t, err = ops.ResolveSessionKeyByTaskID(ctx, taskID)
	return
}
