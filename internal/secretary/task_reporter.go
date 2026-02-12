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
// results to the secretary agent as a user-role system message.
func (s *Service) StartTaskStatusReporter(ctx context.Context, hub *events.Hub) func() {
	if s == nil || s.chat == nil || hub == nil {
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
		lastReported := make(map[string]tasks.Status)
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
				if !ok || !isTaskStatusReportable(task.Status) {
					continue
				}
				if prev, exists := lastReported[task.ID]; exists && prev == task.Status {
					continue
				}
				lastReported[task.ID] = task.Status
				_, _ = s.Send(reportCtx, buildTaskStatusSystemUserPrompt(task))
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
