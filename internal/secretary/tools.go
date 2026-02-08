package secretary

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
)

type taskSummary struct {
	ID        string           `json:"id"`
	Status    tasks.Status     `json:"status"`
	Worker    tasks.WorkerType `json:"worker_type"`
	Prompt    string           `json:"prompt"`
	WorkDir   string           `json:"workdir"`
	UpdatedAt time.Time        `json:"updated_at"`
	CreatedAt time.Time        `json:"created_at"`
}

func newToolRegistry(store *tasks.Store) *agentsdk.ToolRegistry {
	reg := agentsdk.NewToolRegistry()

	_ = reg.Register("system_info", func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
		_ = ctx
		_ = call
		return systeminfo.Snapshot(), nil
	})

	_ = reg.Register("tasks_count", func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
		if store == nil {
			return nil, errors.New("tasks store not configured")
		}
		statusFilter := strings.ToLower(strings.TrimSpace(call.Fields["status"]))
		if statusFilter != "" && !isKnownTaskStatus(statusFilter) {
			return nil, fmt.Errorf("unknown status %q (allowed: %s)", statusFilter, strings.Join(knownTaskStatusesList, ", "))
		}
		includeDeleted := parseBool(call.Fields["include_deleted"])

		counts, total, err := store.CountByStatus(ctx, tasks.ListTasksOptions{IncludeDeleted: includeDeleted})
		if err != nil {
			return nil, err
		}

		by := map[string]int{}
		for st, n := range counts {
			by[string(st)] = n
		}

		if statusFilter != "" {
			filtered := by[statusFilter]
			return map[string]any{
				"total":     filtered,
				"by_status": map[string]int{statusFilter: filtered},
			}, nil
		}

		return map[string]any{
			"total":     total,
			"by_status": by,
		}, nil
	})

	_ = reg.Register("tasks_list", func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
		if store == nil {
			return nil, errors.New("tasks store not configured")
		}
		limit := parseInt(call.Fields["limit"], 50)
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		includeDeleted := parseBool(call.Fields["include_deleted"])

		list, err := store.ListTasksWithOptions(ctx, limit, tasks.ListTasksOptions{IncludeDeleted: includeDeleted})
		if err != nil {
			return nil, err
		}

		out := make([]taskSummary, 0, len(list))
		for _, t := range list {
			out = append(out, taskSummary{
				ID:        t.ID,
				Status:    t.Status,
				Worker:    t.WorkerType,
				Prompt:    truncateDisplay(strings.TrimSpace(t.Prompt), 240),
				WorkDir:   t.WorkDir,
				UpdatedAt: t.UpdatedAt,
				CreatedAt: t.CreatedAt,
			})
		}
		return map[string]any{"tasks": out}, nil
	})

	reg.OnMissing = func(ctx context.Context, call agentsdk.ToolCall) (any, error) {
		_ = ctx
		name := strings.TrimSpace(call.Name)
		if name == "" {
			return nil, agentsdk.ErrToolNotFound
		}
		return nil, agentsdk.ErrToolNotFound
	}

	return reg
}

var knownTaskStatusesList = []string{
	string(tasks.StatusQueued),
	string(tasks.StatusWaiting),
	string(tasks.StatusRunning),
	string(tasks.StatusSucceeded),
	string(tasks.StatusFailed),
	string(tasks.StatusCanceled),
	string(tasks.StatusInterrupted),
	string(tasks.StatusBlocked),
}

var knownTaskStatuses = func() map[string]struct{} {
	out := make(map[string]struct{}, len(knownTaskStatusesList))
	for _, st := range knownTaskStatusesList {
		s := strings.ToLower(strings.TrimSpace(st))
		if s == "" {
			continue
		}
		out[s] = struct{}{}
	}
	return out
}()

func isKnownTaskStatus(s string) bool {
	_, ok := knownTaskStatuses[strings.ToLower(strings.TrimSpace(s))]
	return ok
}

func parseBool(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func parseInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func truncateDisplay(s string, max int) string {
	return truncateRunes(s, max)
}
