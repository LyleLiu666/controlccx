package secretary

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"controlccx/internal/chat"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
)

// StartTaskStatusReporter subscribes to task.updated events and appends proactive
// secretary updates when a task reaches a terminal/blocked status.
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
				_ = s.appendAssistantReport(reportCtx, buildTaskStatusReport(task))
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

func buildTaskStatusReport(task tasks.Task) string {
	taskID := strings.TrimSpace(task.ID)
	worker := strings.TrimSpace(string(task.WorkerType))
	prompt := truncateRunes(strings.TrimSpace(task.Prompt), 120)
	reason := strings.TrimSpace(task.Error)
	if reason == "" {
		reason = strings.TrimSpace(task.FinishReason)
	}

	lines := []string{
		"任务进展汇报：",
		"任务 ID: " + taskID,
		"当前状态: " + humanTaskStatus(task.Status),
	}
	if worker != "" {
		lines = append(lines, "Worker: "+worker)
	}
	if prompt != "" {
		lines = append(lines, "任务摘要: "+prompt)
	}
	if reason != "" && task.Status != tasks.StatusSucceeded {
		lines = append(lines, "原因: "+truncateRunes(reason, 160))
	}
	lines = append(lines, "如需我继续跟进，回复：查看任务 "+taskID)
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (s *Service) appendAssistantReport(ctx context.Context, content string) error {
	if s == nil || s.chat == nil {
		return nil
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if _, err := s.chat.Append(ctx, chat.RoleAssistant, content); err != nil {
		return err
	}
	_ = s.chat.PruneKeepLast(ctx, 2000)
	return nil
}
