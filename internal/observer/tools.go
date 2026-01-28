package observer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"controlccx/internal/chat"
	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
)

type ToolFunc struct {
	ToolName        string
	ToolDescription string
	Fn              func(ctx context.Context, args map[string]any) (any, error)
}

func (t ToolFunc) Name() string        { return t.ToolName }
func (t ToolFunc) Description() string { return t.ToolDescription }
func (t ToolFunc) Run(ctx context.Context, args map[string]any) (any, error) {
	if t.Fn == nil {
		return nil, errors.New("tool has no handler")
	}
	return t.Fn(ctx, args)
}

type TaskSummary struct {
	ID         string           `json:"id"`
	SessionID  string           `json:"session_id"`
	WorkerType tasks.WorkerType `json:"worker_type"`
	Status     tasks.Status     `json:"status"`
	Prompt     string           `json:"prompt"`
	WorkDir    string           `json:"workdir"`
	Score      int              `json:"score"`
	Stderr     int              `json:"stderr_count"`
	UpdatedAt  time.Time        `json:"updated_at"`
	CreatedAt  time.Time        `json:"created_at"`
}

type toolEnv struct {
	tasks *tasks.Store
	chat  *chat.Store
}

func (e toolEnv) listTasks(ctx context.Context, limit int) ([]TaskSummary, error) {
	if e.tasks == nil {
		return nil, errors.New("tasks store not configured")
	}
	items, err := e.tasks.ListTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TaskSummary, 0, len(items))
	for _, t := range items {
		out = append(out, TaskSummary{
			ID:         t.ID,
			SessionID:  strings.TrimSpace(t.SessionID),
			WorkerType: t.WorkerType,
			Status:     t.Status,
			Prompt:     truncateDisplay(strings.TrimSpace(t.Prompt), 240),
			WorkDir:    t.WorkDir,
			Score:      t.Score,
			Stderr:     t.StderrCount,
			UpdatedAt:  t.UpdatedAt,
			CreatedAt:  t.CreatedAt,
		})
	}
	return out, nil
}

func truncateDisplay(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func intArg(args map[string]any, key string, def int, min int, max int) int {
	if args == nil {
		return clamp(def, min, max)
	}
	v, ok := args[key]
	if !ok || v == nil {
		return clamp(def, min, max)
	}
	switch n := v.(type) {
	case float64:
		return clamp(int(n), min, max)
	case int:
		return clamp(n, min, max)
	case int64:
		return clamp(int(n), min, max)
	case string:
		// Ignore non-numeric strings (keep default).
		return clamp(def, min, max)
	default:
		return clamp(def, min, max)
	}
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func boolArg(args map[string]any, key string, def bool) bool {
	if args == nil {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		s := strings.TrimSpace(strings.ToLower(b))
		if s == "1" || s == "true" || s == "yes" {
			return true
		}
		if s == "0" || s == "false" || s == "no" {
			return false
		}
		return def
	case float64:
		return b != 0
	case int:
		return b != 0
	case int64:
		return b != 0
	default:
		return def
	}
}

func clamp(v, min, max int) int {
	if max > 0 && v > max {
		v = max
	}
	if min > 0 && v < min {
		v = min
	}
	return v
}

func (s *Service) agentTools() map[string]Tool {
	env := toolEnv{tasks: s.Store, chat: s.Chat}

	tools := map[string]Tool{
		"system_info": ToolFunc{
			ToolName:        "system_info",
			ToolDescription: "获取服务器系统信息快照。参数：{}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				return systeminfo.Snapshot(), nil
			},
		},
		"tasks_count": ToolFunc{
			ToolName:        "tasks_count",
			ToolDescription: "统计任务数量（可按 status 过滤）。参数：{status?: string}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				statusFilter := strings.TrimSpace(stringArg(args, "status"))

				all, err := s.Store.ListTasks(ctx, 500)
				if err != nil {
					return nil, err
				}
				counts := map[string]int{}
				total := 0
				for _, t := range all {
					st := string(t.Status)
					if statusFilter != "" && st != statusFilter {
						continue
					}
					counts[st]++
					total++
				}
				return map[string]any{
					"total":     total,
					"by_status": counts,
				}, nil
			},
		},
		"tasks_most_problematic": ToolFunc{
			ToolName:        "tasks_most_problematic",
			ToolDescription: "获取当前最需要关注的任务（按 score/时间排序）。参数：{}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				all, err := s.Store.ListTasks(ctx, 500)
				if err != nil {
					return nil, err
				}
				if len(all) == 0 {
					return map[string]any{"task": nil}, nil
				}
				sort.SliceStable(all, func(i, j int) bool {
					if all[i].Score == all[j].Score {
						return all[i].UpdatedAt.After(all[j].UpdatedAt)
					}
					return all[i].Score > all[j].Score
				})
				t := all[0]
				return map[string]any{
					"task": TaskSummary{
						ID:         t.ID,
						SessionID:  strings.TrimSpace(t.SessionID),
						WorkerType: t.WorkerType,
						Status:     t.Status,
						Prompt:     truncateDisplay(strings.TrimSpace(t.Prompt), 240),
						WorkDir:    t.WorkDir,
						Score:      t.Score,
						Stderr:     t.StderrCount,
						UpdatedAt:  t.UpdatedAt,
						CreatedAt:  t.CreatedAt,
					},
				}, nil
			},
		},
		"tasks_list": ToolFunc{
			ToolName:        "tasks_list",
			ToolDescription: "列出最近的任务（包含 prompt 摘要）。参数：{limit?: number (1..500)}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				limit := intArg(args, "limit", 50, 1, 500)
				tasks, err := env.listTasks(ctx, limit)
				if err != nil {
					return nil, err
				}
				return map[string]any{"tasks": tasks}, nil
			},
		},
		"task_logs": ToolFunc{
			ToolName:        "task_logs",
			ToolDescription: "列出指定任务的日志。参数：{task_id: string, after?: number, limit?: number (1..2000)}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				taskID := stringArg(args, "task_id")
				if taskID == "" {
					taskID = stringArg(args, "id")
				}
				taskID, err := s.resolveTaskID(ctx, taskID)
				if err != nil {
					return nil, err
				}

				after := int64(intArg(args, "after", 0, 0, 1<<31-1))
				limit := intArg(args, "limit", 200, 1, 2000)
				logs, err := s.Store.ListLogs(ctx, taskID, after, limit)
				if err != nil {
					return nil, err
				}
				// Truncate each log line to avoid huge payloads.
				const maxLine = 2000
				for i := range logs {
					logs[i].Message = truncateDisplay(logs[i].Message, maxLine)
				}
				return map[string]any{"task_id": taskID, "logs": logs}, nil
			},
		},
		"task_resume": ToolFunc{
			ToolName:        "task_resume",
			ToolDescription: "恢复/继续某个任务所属的会话：创建一个新的 resume run 并启动。参数：{task_id: string, prompt?: string, unsafe_automation?: boolean}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				if s.Runner == nil {
					return nil, errors.New("task runner not configured")
				}

				taskID := stringArg(args, "task_id")
				if taskID == "" {
					taskID = stringArg(args, "id")
				}
				taskID, err := s.resolveTaskID(ctx, taskID)
				if err != nil {
					return nil, err
				}

				prev, err := s.Store.GetTask(ctx, taskID)
				if err != nil {
					return nil, err
				}

				if prev.SessionDeletedAt != nil {
					return nil, fmt.Errorf("session is deleted; cannot resume (task_id=%s)", prev.ID)
				}

				sid := strings.TrimSpace(prev.SessionID)
				if sid == "" {
					return nil, fmt.Errorf("task has no session_id to resume (task_id=%s)", prev.ID)
				}

				// Avoid overlapping runs for the same session.
				all, err := s.Store.ListTasks(ctx, 500)
				if err != nil {
					return nil, err
				}
				for _, t := range all {
					if strings.TrimSpace(t.SessionID) != sid {
						continue
					}
					if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued {
						return nil, fmt.Errorf("session already has a running task (task_id=%s status=%s)", t.ID, t.Status)
					}
				}

				prompt := stringArg(args, "prompt")
				if prompt == "" {
					prompt = "continue"
				}
				unsafe := boolArg(args, "unsafe_automation", prev.UnsafeAutomation)

				next, err := s.Store.CreateTask(ctx, tasks.CreateTaskInput{
					WorkerType:        prev.WorkerType,
					Mode:              tasks.ModeResume,
					UnsafeAutomation:  unsafe,
					Prompt:            prompt,
					WorkDir:           prev.WorkDir,
					SessionID:         sid,
					Warning:           prev.Warning,
				})
				if err != nil {
					return nil, err
				}

				if err := s.Runner.Start(ctx, next.ID); err != nil {
					_ = s.Store.FinishTask(context.Background(), next.ID, tasks.FinishTaskInput{
						Status:     tasks.StatusFailed,
						ExitCode:   nil,
						Error:      err.Error(),
						SessionID:  "",
						FinishedAt: time.Now().UTC(),
					})
					return nil, err
				}

				return map[string]any{
					"ok": true,
					"task": map[string]any{
						"id":          next.ID,
						"session_id":  strings.TrimSpace(next.SessionID),
						"worker_type": next.WorkerType,
						"status":      next.Status,
						"prompt":      truncateDisplay(strings.TrimSpace(next.Prompt), 240),
						"workdir":     next.WorkDir,
						"unsafe":      next.UnsafeAutomation,
					},
				}, nil
			},
		},
		"task_cancel": ToolFunc{
			ToolName:        "task_cancel",
			ToolDescription: "取消一个正在执行的任务。参数：{task_id: string}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Runner == nil {
					return nil, errors.New("task runner not configured")
				}
				taskID := stringArg(args, "task_id")
				if taskID == "" {
					taskID = stringArg(args, "id")
				}
				taskID, err := s.resolveTaskID(ctx, taskID)
				if err != nil {
					return nil, err
				}
				ok := s.Runner.Cancel(taskID)
				return map[string]any{"ok": ok, "task_id": taskID}, nil
			},
		},
	}

	taskOutputStats := ToolFunc{
		ToolName:        "task_output_stats",
		ToolDescription: "获取任务最新输出 + 字数统计。参数：{task_id: string, max_chars?: number}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
		Fn: func(ctx context.Context, args map[string]any) (any, error) {
			if s.Store == nil {
				return nil, errors.New("tasks store not configured")
			}
			taskID := stringArg(args, "task_id")
			if taskID == "" {
				taskID = stringArg(args, "id")
			}
			taskID, err := s.resolveTaskID(ctx, taskID)
			if err != nil {
				return nil, err
			}
			t, err := s.Store.GetTask(ctx, taskID)
			if err != nil {
				return nil, err
			}
			out, err := s.latestTaskOutput(ctx, t)
			if err != nil {
				return nil, err
			}
			maxChars := intArg(args, "max_chars", 2000, 1, 20000)
			preview := truncateDisplay(out, maxChars)
			st := computeLengthStat(out)
			return map[string]any{
				"task": map[string]any{
					"id":          t.ID,
					"session_id":  strings.TrimSpace(t.SessionID),
					"worker_type": t.WorkerType,
					"status":      t.Status,
					"prompt":      truncateDisplay(strings.TrimSpace(t.Prompt), 240),
					"workdir":     t.WorkDir,
					"score":       t.Score,
					"stderr":      t.StderrCount,
				},
				"output_preview": preview,
				"stats": map[string]any{
					"chars_no_space": st.NonSpaceRunes,
					"chars":          st.Runes,
					"words":          st.Words,
				},
			}, nil
		},
	}

	tools["task_output_stats"] = taskOutputStats
	// Backward-compatible alias (LLM may guess names).
	tools["task_output_status"] = ToolFunc{
		ToolName:        "task_output_status",
		ToolDescription: "（别名）同 task_output_stats。参数：{task_id: string, max_chars?: number}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
		Fn:              taskOutputStats.Fn,
	}

	if s.Chat != nil {
		tools["chat_history"] = ToolFunc{
			ToolName:        "chat_history",
			ToolDescription: "列出最近的对话消息。参数：{after?: number, limit?: number (1..500)}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				after := int64(intArg(args, "after", 0, 0, 1<<31-1))
				limit := intArg(args, "limit", 50, 1, 500)
				msgs, err := s.Chat.List(ctx, after, limit)
				if err != nil {
					return nil, err
				}
				return map[string]any{"messages": msgs}, nil
			},
		}
	}

	return tools
}

func (s *Service) resolveTaskID(ctx context.Context, idOrPrefix string) (string, error) {
	idOrPrefix = strings.TrimSpace(idOrPrefix)
	if idOrPrefix == "" {
		return "", fmt.Errorf("task_id is required")
	}
	// Fast path: full UUID.
	if len(idOrPrefix) >= 32 {
		// Allow callers to pass full ID directly.
		if len(idOrPrefix) == 36 && strings.Count(idOrPrefix, "-") == 4 {
			return idOrPrefix, nil
		}
	}

	if s.Store == nil {
		return "", errors.New("tasks store not configured")
	}
	all, err := s.Store.ListTasks(ctx, 500)
	if err != nil {
		return "", err
	}
	prefix := strings.ToLower(idOrPrefix)
	var match string
	var ambiguous bool
	for _, t := range all {
		if strings.HasPrefix(strings.ToLower(t.ID), prefix) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.SessionID)), prefix) {
			if match != "" && match != t.ID {
				ambiguous = true
				break
			}
			match = t.ID
		}
	}
	if ambiguous {
		return "", fmt.Errorf("ambiguous task_id prefix: %s", idOrPrefix)
	}
	if match != "" {
		return match, nil
	}

	// Best-effort: treat input as a keyword in prompt (useful when users refer to a task by name).
	query := strings.ToLower(strings.TrimSpace(idOrPrefix))
	type candidate struct {
		id     string
		prompt string
	}
	var cands []candidate
	for _, t := range all {
		p := strings.ToLower(strings.TrimSpace(t.Prompt))
		if p == "" {
			continue
		}
		if strings.Contains(p, query) {
			cands = append(cands, candidate{
				id:     t.ID,
				prompt: truncateDisplay(strings.ReplaceAll(strings.TrimSpace(t.Prompt), "\n", " "), 80),
			})
		}
	}
	if len(cands) == 1 {
		return cands[0].id, nil
	}
	if len(cands) > 1 {
		parts := make([]string, 0, minInt(len(cands), 5))
		for i := 0; i < len(cands) && i < 5; i++ {
			id := cands[i].id
			if len(id) > 8 {
				id = id[:8]
			}
			if cands[i].prompt != "" {
				parts = append(parts, fmt.Sprintf("%s(%s)", id, cands[i].prompt))
			} else {
				parts = append(parts, id)
			}
		}
		more := ""
		if len(cands) > 5 {
			more = fmt.Sprintf(" (+%d more)", len(cands)-5)
		}
		return "", fmt.Errorf("ambiguous task reference: %s; candidates: %s%s", idOrPrefix, strings.Join(parts, ", "), more)
	}

	return "", fmt.Errorf("task_id not found: %s", idOrPrefix)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
