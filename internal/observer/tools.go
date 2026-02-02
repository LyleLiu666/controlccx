package observer

import (
	"context"
	"encoding/json"
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
					WorkerType:       prev.WorkerType,
					Mode:             tasks.ModeResume,
					UnsafeAutomation: unsafe,
					Prompt:           prompt,
					WorkDir:          prev.WorkDir,
					SessionID:        sid,
					Warning:          prev.Warning,
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
		"session_continue": ToolFunc{
			ToolName:        "session_continue",
			ToolDescription: "继续一个会话：根据最新 run 自动选择 resume 或 rehydrate（当检测到 No conversation found 时）。参数：{key?: string, task_id?: string, prompt?: string, unsafe_automation?: boolean}。key 为 c:/s:/t: 会话键；task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				if s.Runner == nil {
					return nil, errors.New("task runner not configured")
				}

				key := strings.TrimSpace(stringArg(args, "key"))
				taskID := strings.TrimSpace(stringArg(args, "task_id"))
				if taskID == "" {
					taskID = strings.TrimSpace(stringArg(args, "id"))
				}

				var (
					conversationID string
				)
				if key != "" {
					cid, err := resolveConversationIDForSessionKey(ctx, s.Store, key)
					if err != nil {
						return nil, err
					}
					conversationID = cid
				} else {
					if taskID == "" {
						return nil, errors.New("session_continue: key or task_id is required")
					}
					resolved, err := s.resolveTaskID(ctx, taskID)
					if err != nil {
						return nil, err
					}
					t, err := s.Store.GetTask(ctx, resolved)
					if err != nil {
						return nil, err
					}
					conversationID = strings.TrimSpace(t.ConversationID)
					if conversationID == "" {
						conversationID = strings.TrimSpace(t.ID)
					}
				}

				runs, err := s.Store.ListTasksByConversationID(ctx, conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
				if err != nil {
					return nil, err
				}
				if len(runs) == 0 {
					return nil, errors.New("session not found")
				}
				// Deterministic: newest first with ID tie-break.
				sort.SliceStable(runs, func(i, j int) bool {
					if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
						return runs[i].ID > runs[j].ID
					}
					return runs[i].CreatedAt.After(runs[j].CreatedAt)
				})

				latest := runs[0]
				if latest.SessionDeletedAt != nil {
					return nil, fmt.Errorf("session is deleted; cannot continue (conversation_id=%s)", strings.TrimSpace(latest.ConversationID))
				}

				for _, t := range runs {
					if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued {
						return nil, fmt.Errorf("session already has a running task (task_id=%s status=%s)", t.ID, t.Status)
					}
				}

				prompt := strings.TrimSpace(stringArg(args, "prompt"))
				if prompt == "" {
					prompt = "continue"
				}
				unsafe := boolArg(args, "unsafe_automation", latest.UnsafeAutomation)

				// If the latest run is blocked, require explicit unsafe_automation=true to proceed.
				if latest.Status == tasks.StatusBlocked && !unsafe {
					return nil, fmt.Errorf("latest run is blocked; set unsafe_automation=true to continue (task_id=%s)", latest.ID)
				}

				shouldRehydrate := latest.Mode == tasks.ModeResume &&
					(isNoConversationFound(latest.Warning) || isNoConversationFound(latest.Error))

				if shouldRehydrate {
					ctxPrompt, err := tasks.BuildRehydratePrompt(ctx, s.Store, conversationID, prompt)
					if err != nil {
						return nil, err
					}
					next, err := s.Store.CreateTask(ctx, tasks.CreateTaskInput{
						WorkerType:            latest.WorkerType,
						Mode:                  tasks.ModeNew,
						ConversationID:        conversationID,
						UnsafeAutomation:      unsafe,
						SafetyPreset:          strings.TrimSpace(latest.SafetyPreset),
						TaskIntent:            strings.TrimSpace(latest.TaskIntent),
						CodexSandbox:          strings.TrimSpace(latest.CodexSandbox),
						CodexApprovalPolicy:   strings.TrimSpace(latest.CodexApprovalPolicy),
						CodexSearch:           latest.CodexSearch,
						ClaudePermissionMode:  strings.TrimSpace(latest.ClaudePermissionMode),
						ClaudeSandbox:         latest.ClaudeSandbox,
						ClaudeWebFetchDomains: append([]string{}, latest.ClaudeWebFetchDomains...),
						Prompt:                ctxPrompt,
						WorkDir:               latest.WorkDir,
						SessionID:             "",
					})
					if err != nil {
						return nil, err
					}
					_, _ = s.Store.AppendLog(ctx, next.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", latest.ID, strings.TrimSpace(latest.SessionID)))

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
							"id":              next.ID,
							"mode":            next.Mode,
							"conversation_id": strings.TrimSpace(next.ConversationID),
							"session_id":      strings.TrimSpace(next.SessionID),
							"worker_type":     next.WorkerType,
							"status":          next.Status,
							"prompt":          truncateDisplay(strings.TrimSpace(next.Prompt), 240),
							"workdir":         next.WorkDir,
							"unsafe":          next.UnsafeAutomation,
							"continued_from":  latest.ID,
						},
					}, nil
				}

				sid := strings.TrimSpace(latest.SessionID)
				if sid == "" {
					return nil, fmt.Errorf("task has no session_id to resume (task_id=%s)", latest.ID)
				}

				next, err := s.Store.CreateTask(ctx, tasks.CreateTaskInput{
					WorkerType:            latest.WorkerType,
					Mode:                  tasks.ModeResume,
					ConversationID:        conversationID,
					UnsafeAutomation:      unsafe,
					SafetyPreset:          strings.TrimSpace(latest.SafetyPreset),
					TaskIntent:            strings.TrimSpace(latest.TaskIntent),
					CodexSandbox:          strings.TrimSpace(latest.CodexSandbox),
					CodexApprovalPolicy:   strings.TrimSpace(latest.CodexApprovalPolicy),
					CodexSearch:           latest.CodexSearch,
					ClaudePermissionMode:  strings.TrimSpace(latest.ClaudePermissionMode),
					ClaudeSandbox:         latest.ClaudeSandbox,
					ClaudeWebFetchDomains: append([]string{}, latest.ClaudeWebFetchDomains...),
					Prompt:                prompt,
					WorkDir:               latest.WorkDir,
					SessionID:             sid,
					Warning:               latest.Warning,
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
						"id":              next.ID,
						"mode":            next.Mode,
						"conversation_id": strings.TrimSpace(next.ConversationID),
						"session_id":      strings.TrimSpace(next.SessionID),
						"worker_type":     next.WorkerType,
						"status":          next.Status,
						"prompt":          truncateDisplay(strings.TrimSpace(next.Prompt), 240),
						"workdir":         next.WorkDir,
						"unsafe":          next.UnsafeAutomation,
						"continued_from":  latest.ID,
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
		"acceptance_classify": ToolFunc{
			ToolName:        "acceptance_classify",
			ToolDescription: "（deterministic）判断某个任务是否属于复杂任务，并给出一句话理由。参数：{task_id: string}。task_id 可为：完整 id / id 前缀 / session_id / prompt 关键词",
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
				prompt := s.bestEffortSessionPrompt(ctx, t)
				complex, reason, signals := classifyComplexityHeuristic(prompt)
				return map[string]any{
					"complex":    complex,
					"reason":     reason,
					"signals":    signals,
					"prompt":     truncateDisplay(strings.TrimSpace(prompt), 240),
					"task_id":    t.ID,
					"session_id": strings.TrimSpace(t.SessionID),
				}, nil
			},
		},
		"acceptance_prepare": ToolFunc{
			ToolName:        "acceptance_prepare",
			ToolDescription: "准备/推进验收状态（按 run_id 变更自动推进 iteration，并 enforce max_iterations）。参数：{task_id: string, max_iterations?: number}。返回：{can_continue:boolean, iteration_advanced:boolean, state:AcceptanceState}",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				runID := stringArg(args, "task_id")
				if runID == "" {
					runID = stringArg(args, "id")
				}
				if runID == "" {
					return nil, errors.New("acceptance_prepare: task_id is required")
				}
				resolved, err := s.resolveTaskID(ctx, runID)
				if err != nil {
					return nil, err
				}
				t, err := s.Store.GetTask(ctx, resolved)
				if err != nil {
					return nil, err
				}
				key := tasks.SessionKeyForTask(t)

				maxIter := intArg(args, "max_iterations", 10, 1, 100)

				prev, ok, err := s.Store.GetAcceptanceState(ctx, key)
				if err != nil {
					return nil, err
				}

				iterationAdvanced := false
				canContinue := true

				if !ok {
					st, err := s.Store.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
						Key:           key,
						Status:        "running",
						Iteration:     1,
						MaxIterations: maxIter,
						CurrentGate:   "contract",
						Summary:       "acceptance started",
						RunID:         t.ID,
					})
					if err != nil {
						return nil, err
					}
					return map[string]any{
						"can_continue":       true,
						"iteration_advanced": false,
						"state":              st,
					}, nil
				}

				// Terminal states: do not proceed.
				switch strings.ToLower(strings.TrimSpace(prev.Status)) {
				case "accepted", "failed":
					return map[string]any{
						"can_continue":       false,
						"iteration_advanced": false,
						"state":              prev,
					}, nil
				}

				next := prev
				next.MaxIterations = maxIter
				if next.Iteration <= 0 {
					next.Iteration = 1
				}

				if strings.TrimSpace(prev.RunID) != "" && strings.TrimSpace(prev.RunID) != t.ID {
					if next.Iteration >= next.MaxIterations {
						canContinue = false
						next.Status = "failed"
						next.Summary = "iteration limit reached"
					} else {
						next.Iteration++
						iterationAdvanced = true
					}
				}
				next.RunID = t.ID
				if strings.TrimSpace(next.Status) == "" {
					next.Status = "running"
				}

				st, err := s.Store.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
					Key:           key,
					Status:        next.Status,
					Iteration:     next.Iteration,
					MaxIterations: next.MaxIterations,
					CurrentGate:   strings.TrimSpace(next.CurrentGate),
					Summary:       strings.TrimSpace(next.Summary),
					PlanJSON:      next.PlanJSON,
					Report:        next.Report,
					RunID:         next.RunID,
				})
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"can_continue":       canContinue,
					"iteration_advanced": iterationAdvanced,
					"state":              st,
				}, nil
			},
		},
		"acceptance_build_contract": ToolFunc{
			ToolName:        "acceptance_build_contract",
			ToolDescription: "（deterministic baseline）从 session prompt 中抽取 objective constraints，并生成 acceptance plan_json（方法论优先、无固定分类）。参数：{task_id: string}。返回：{plan_json:string, plan:object}",
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
				prompt := s.bestEffortSessionPrompt(ctx, t)
				plan := buildAcceptancePlanHeuristic(prompt)
				b, err := json.Marshal(plan)
				if err != nil {
					return nil, fmt.Errorf("acceptance_build_contract: %w", err)
				}
				return map[string]any{
					"plan_json": string(b),
					"plan":      plan,
					"prompt":    truncateDisplay(strings.TrimSpace(prompt), 300),
				}, nil
			},
		},
		"acceptance_evaluate_objectives": ToolFunc{
			ToolName:        "acceptance_evaluate_objectives",
			ToolDescription: "（deterministic）按 plan_json 评估客观标准（words/sections/字数等），输出 pass/fail + 证据。参数：{task_id: string, plan_json: string}",
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

				planJSON := stringArg(args, "plan_json")
				plan, err := parseAcceptancePlanJSON(planJSON)
				if err != nil {
					return nil, err
				}

				out, err := s.latestTaskOutput(ctx, t)
				if err != nil {
					return nil, err
				}
				st := computeLengthStat(out)
				hs := computeHeadingStat(out)

				results := make([]AcceptanceObjectiveResult, 0, len(plan.ObjectiveCriteria))
				for _, c := range plan.ObjectiveCriteria {
					measured, unit, ok := acceptanceMeasureForMethod(c.Method, st, hs)
					if !ok {
						results = append(results, AcceptanceObjectiveResult{
							ID:     strings.TrimSpace(c.ID),
							Title:  strings.TrimSpace(c.Title),
							Method: strings.TrimSpace(c.Method),
							Pass:   false,
							Min:    c.Min,
							Max:    c.Max,
							Unit:   strings.TrimSpace(c.Unit),
							Note:   "unsupported objective method",
						})
						continue
					}
					pass := true
					if c.Min > 0 && measured < c.Min {
						pass = false
					}
					if c.Max > 0 && measured > c.Max {
						pass = false
					}

					title := strings.TrimSpace(c.Title)
					if title == "" {
						title = strings.TrimSpace(c.ID)
					}
					results = append(results, AcceptanceObjectiveResult{
						ID:       strings.TrimSpace(c.ID),
						Title:    title,
						Method:   strings.TrimSpace(c.Method),
						Pass:     pass,
						Measured: measured,
						Min:      c.Min,
						Max:      c.Max,
						Unit:     unit,
						Evidence: []AcceptanceEvidenceRef{
							{Kind: "count", Ref: fmt.Sprintf("%s=%d", unit, measured), Note: fmt.Sprintf("min=%d max=%d", c.Min, c.Max)},
						},
					})
				}

				return map[string]any{
					"task_id": t.ID,
					"results": results,
					"stats": map[string]any{
						"chars_no_space": st.NonSpaceRunes,
						"chars":          st.Runes,
						"words":          st.Words,
						"sections":       hs.HeadingLines,
					},
				}, nil
			},
		},
		"acceptance_get": ToolFunc{
			ToolName:        "acceptance_get",
			ToolDescription: "获取某个会话（conversation/session）的验收状态（Acceptance Gates）。参数：{key?: string, task_id?: string}。优先使用 key（形如 c:<conversation_id> / s:<session_id> / t:<task_id>）。",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}
				key := stringArg(args, "key")
				if key == "" {
					taskID := stringArg(args, "task_id")
					if taskID == "" {
						taskID = stringArg(args, "id")
					}
					if taskID != "" {
						resolved, err := s.resolveTaskID(ctx, taskID)
						if err != nil {
							return nil, err
						}
						t, err := s.Store.GetTask(ctx, resolved)
						if err != nil {
							return nil, err
						}
						key = tasks.SessionKeyForTask(t)
					}
				}
				if key == "" {
					return nil, errors.New("acceptance_get: key or task_id is required")
				}
				st, ok, err := s.Store.GetAcceptanceState(ctx, key)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": ok, "state": st}, nil
			},
		},
		"acceptance_update": ToolFunc{
			ToolName:        "acceptance_update",
			ToolDescription: "更新/写入某个 session 的验收状态（Acceptance Gates），用于 UI 可见的进度与报告。参数：{key?: string, task_id?: string, status?: string, iteration?: number, max_iterations?: number, current_gate?: string, summary?: string, plan_json?: string, report?: string, run_id?: string}。未提供的字段将保留原值。",
			Fn: func(ctx context.Context, args map[string]any) (any, error) {
				if s.Store == nil {
					return nil, errors.New("tasks store not configured")
				}

				// Resolve session key.
				key := stringArg(args, "key")
				runID := stringArg(args, "run_id")
				if runID == "" {
					runID = stringArg(args, "task_id")
				}
				if runID == "" {
					runID = stringArg(args, "id")
				}
				if key == "" && runID != "" {
					resolved, err := s.resolveTaskID(ctx, runID)
					if err != nil {
						return nil, err
					}
					t, err := s.Store.GetTask(ctx, resolved)
					if err != nil {
						return nil, err
					}
					key = tasks.SessionKeyForTask(t)
					if runID == "" {
						runID = t.ID
					}
				}
				if key == "" {
					return nil, errors.New("acceptance_update: key or task_id is required")
				}

				// Merge with existing state so callers can send partial updates.
				prev, ok, err := s.Store.GetAcceptanceState(ctx, key)
				if err != nil {
					return nil, err
				}
				next := tasks.UpsertAcceptanceStateInput{
					Key:           key,
					Status:        "",
					Iteration:     0,
					MaxIterations: 0,
					CurrentGate:   "",
					Summary:       "",
					PlanJSON:      "",
					Report:        "",
					RunID:         "",
				}
				if ok {
					next.Status = prev.Status
					next.Iteration = prev.Iteration
					next.MaxIterations = prev.MaxIterations
					next.CurrentGate = prev.CurrentGate
					next.Summary = prev.Summary
					next.PlanJSON = prev.PlanJSON
					next.Report = prev.Report
					next.RunID = prev.RunID
				}

				if _, exists := args["status"]; exists {
					next.Status = stringArg(args, "status")
				}
				if _, exists := args["iteration"]; exists {
					next.Iteration = intArg(args, "iteration", next.Iteration, 0, 1<<31-1)
				}
				if _, exists := args["max_iterations"]; exists {
					next.MaxIterations = intArg(args, "max_iterations", next.MaxIterations, 0, 1<<31-1)
				}
				if _, exists := args["current_gate"]; exists {
					next.CurrentGate = stringArg(args, "current_gate")
				}
				if _, exists := args["summary"]; exists {
					next.Summary = stringArg(args, "summary")
				}
				if _, exists := args["plan_json"]; exists {
					next.PlanJSON = stringArg(args, "plan_json")
				}
				if _, exists := args["report"]; exists {
					next.Report = stringArg(args, "report")
				}
				if _, exists := args["run_id"]; exists {
					next.RunID = stringArg(args, "run_id")
				} else if runID != "" && strings.TrimSpace(next.RunID) == "" {
					next.RunID = runID
				}

				st, err := s.Store.UpsertAcceptanceState(ctx, next)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "state": st}, nil
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
			hs := computeHeadingStat(out)
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
					"chars_no_space":    st.NonSpaceRunes,
					"chars":             st.Runes,
					"words":             st.Words,
					"heading_lines":     hs.HeadingLines,
					"headings_markdown": hs.MarkdownHeading,
					"headings_numbered": hs.NumberedHeading,
					"headings_chinese":  hs.ChineseHeading,
					// Alias: many acceptance checks talk about "sections".
					"sections": hs.HeadingLines,
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
				var (
					msgs []chat.Message
					err  error
				)
				if after > 0 {
					msgs, err = s.Chat.List(ctx, after, limit)
				} else {
					msgs, err = s.Chat.Tail(ctx, limit)
				}
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

func resolveConversationIDForSessionKey(ctx context.Context, store *tasks.Store, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("session key is required")
	}
	if store == nil {
		return "", errors.New("tasks store not configured")
	}
	if strings.HasPrefix(key, "c:") {
		cid := strings.TrimSpace(strings.TrimPrefix(key, "c:"))
		if cid == "" {
			return "", errors.New("conversation_id is required")
		}
		return cid, nil
	}
	if strings.HasPrefix(key, "t:") {
		taskID := strings.TrimSpace(strings.TrimPrefix(key, "t:"))
		if taskID == "" {
			return "", errors.New("task_id is required")
		}
		t, err := store.GetTask(ctx, taskID)
		if err != nil {
			return "", fmt.Errorf("task not found: %w", err)
		}
		if cid := strings.TrimSpace(t.ConversationID); cid != "" {
			return cid, nil
		}
		return strings.TrimSpace(t.ID), nil
	}
	if strings.HasPrefix(key, "s:") {
		sid := strings.TrimSpace(strings.TrimPrefix(key, "s:"))
		if sid == "" {
			return "", errors.New("session_id is required")
		}
		if cid, ok, err := store.ConversationIDForSessionID(ctx, sid); err != nil {
			return "", err
		} else if ok {
			return cid, nil
		}
		return "", errors.New("session not found")
	}
	return "", errors.New("invalid session key (expected c:/s:/t:)")
}

func isNoConversationFound(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "no conversation found") {
		return false
	}
	return strings.Contains(lower, "session")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
