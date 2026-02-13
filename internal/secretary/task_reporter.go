package secretary

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

// StartTaskStatusReporter subscribes to task.updated events and forwards task
// results/attention signals to the secretary agent as a user-role system message.
func (s *Service) StartTaskStatusReporter(ctx context.Context, hub *events.Hub) func() {
	if s == nil || s.tasks == nil || s.chat == nil || hub == nil {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	eventsCh, unsubscribe := hub.Subscribe(256)
	reportCtx, cancel := context.WithCancel(ctx)
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			unsubscribe()
		})
	}

	go func() {
		lastStatus := make(map[string]tasks.Status)
		for {
			select {
			case <-reportCtx.Done():
				return
			case evt, ok := <-eventsCh:
				if !ok {
					return
				}
				if strings.TrimSpace(evt.Type) != "task.updated" {
					continue
				}
				task, ok := decodeTaskFromEventPayload(evt.Payload)
				if !ok {
					continue
				}

				if prev, exists := lastStatus[task.ID]; exists && prev == task.Status {
					continue
				}
				lastStatus[task.ID] = task.Status

				switch task.Status {
				case tasks.StatusAwaitingApproval:
					pending, err := s.tasks.ListApprovalRequestsByTask(reportCtx, task.ID, tasks.ListApprovalRequestsOptions{
						Status: tasks.ApprovalStatusPending,
						Limit:  5,
					})
					if err != nil {
						pending = nil
					}
					_, _ = s.Send(reportCtx, buildTaskAwaitingApprovalSystemUserPrompt(task, pending))
				default:
					if !isTaskStatusReportable(task.Status) {
						continue
					}
					_, _ = s.Send(reportCtx, buildTaskStatusSystemUserPrompt(task))
				}
			}
		}
	}()

	return stop
}

func decodeTaskFromEventPayload(payload any) (tasks.Task, bool) {
	switch v := payload.(type) {
	case tasks.Task:
		v.ID = strings.TrimSpace(v.ID)
		return v, v.ID != ""
	case *tasks.Task:
		if v == nil {
			return tasks.Task{}, false
		}
		out := *v
		out.ID = strings.TrimSpace(out.ID)
		return out, out.ID != ""
	default:
		raw, err := json.Marshal(payload)
		if err != nil || len(raw) == 0 {
			return tasks.Task{}, false
		}
		var out tasks.Task
		if err := json.Unmarshal(raw, &out); err != nil {
			return tasks.Task{}, false
		}
		out.ID = strings.TrimSpace(out.ID)
		return out, out.ID != ""
	}
}

func isTaskStatusReportable(status tasks.Status) bool {
	switch status {
	case tasks.StatusSucceeded, tasks.StatusFailed, tasks.StatusInterrupted, tasks.StatusCanceled, tasks.StatusBlocked:
		return true
	default:
		return false
	}
}

func humanTaskStatus(status tasks.Status) string {
	switch status {
	case tasks.StatusSucceeded:
		return "已完成"
	case tasks.StatusFailed:
		return "执行失败"
	case tasks.StatusInterrupted:
		return "已中断"
	case tasks.StatusCanceled:
		return "已取消"
	case tasks.StatusBlocked:
		return "受阻"
	default:
		return strings.TrimSpace(string(status))
	}
}

func buildTaskAwaitingApprovalSystemUserPrompt(task tasks.Task, pending []tasks.ApprovalRequest) string {
	taskID := strings.TrimSpace(task.ID)
	worker := strings.TrimSpace(string(task.WorkerType))
	prompt := truncateRunes(strings.TrimSpace(task.Prompt), 120)

	lines := []string{
		"【系统消息】",
		"这是一条由 ControlCCX 自动注入的系统消息，不是用户直接输入。",
		"检测到该 run 正在等待审批（awaiting_approval）。请你基于上下文判断是否应批准，并尽量让 run 继续执行。",
		"你可以调用工具 task_approval_decide 来提交 approve/deny 决策；如果你认为需要用户确认，请向用户说明原因并等待用户操作。",
		"",
		"待审批信息：",
		"- task_id: " + taskID,
		"- status: awaiting_approval",
	}
	if worker != "" {
		lines = append(lines, "- worker: "+worker)
	}
	if prompt != "" {
		lines = append(lines, "- task_summary: "+prompt)
	}
	if len(pending) == 0 {
		lines = append(lines, "- pending_approvals: []")
		lines = append(lines, "")
		lines = append(lines, "注意：当前未查询到 pending approval_requests（可能已被处理/过期或数据未落库）。你仍可尝试根据 task_id 调用 task_approval_decide。")
		lines = append(lines, "")
		lines = append(lines, "请直接开始处理。")
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	lines = append(lines, "- pending_approvals:")
	for _, ar := range pending {
		approvalID := strings.TrimSpace(ar.ID)
		actionType := strings.TrimSpace(ar.ActionType)
		riskLevel := strings.TrimSpace(string(ar.RiskLevel))
		workdir := strings.TrimSpace(ar.WorkDir)
		summary := strings.Join(strings.Fields(strings.TrimSpace(ar.Summary)), " ")
		raw := strings.Join(strings.Fields(strings.TrimSpace(string(ar.Raw))), " ")

		if approvalID == "" {
			continue
		}
		lines = append(lines, "  - approval_id: "+approvalID)
		if actionType != "" {
			lines = append(lines, "    action_type: "+actionType)
		}
		if riskLevel != "" {
			lines = append(lines, "    risk_level: "+riskLevel)
		}
		if workdir != "" {
			lines = append(lines, "    workdir: "+truncateRunes(workdir, 160))
		}
		if summary != "" {
			lines = append(lines, "    summary: "+truncateRunes(summary, 160))
		}
		if raw != "" && raw != "{}" {
			lines = append(lines, "    raw: "+truncateRunes(raw, 240))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "请直接开始处理。")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func buildTaskStatusSystemUserPrompt(task tasks.Task) string {
	taskID := strings.TrimSpace(task.ID)
	worker := strings.TrimSpace(string(task.WorkerType))
	prompt := truncateRunes(strings.TrimSpace(task.Prompt), 120)
	warning := strings.Join(strings.Fields(strings.TrimSpace(task.Warning)), " ")
	reason := strings.TrimSpace(task.Error)
	if reason == "" {
		reason = strings.TrimSpace(task.FinishReason)
	}
	reasonCompact := strings.Join(strings.Fields(strings.TrimSpace(reason)), " ")

	lines := []string{
		"【系统消息】",
		"这是一条由 ControlCCX 自动注入的系统消息，不是用户直接输入。",
		"请你向用户汇报结果：简洁说明任务状态、结果与建议下一步。",
		"执行要求：不要调用工具，不要创建/修改任务，只做结果汇报。",
		"",
		"任务执行结果：",
		"- task_id: " + taskID,
		"- status: " + humanTaskStatus(task.Status),
	}
	if worker != "" {
		lines = append(lines, "- worker: "+worker)
	}
	if prompt != "" {
		lines = append(lines, "- task_summary: "+prompt)
	}
	if reason != "" && task.Status != tasks.StatusSucceeded {
		lines = append(lines, "- reason: "+truncateRunes(reasonCompact, 160))
	}
	if warning != "" && (task.Status == tasks.StatusSucceeded || warning != reasonCompact) {
		lines = append(lines, "- warning: "+truncateRunes(warning, 160))
	}
	lines = append(lines, "")
	lines = append(lines, "请直接面向用户输出中文汇报。")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
