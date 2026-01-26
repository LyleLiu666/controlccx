package observer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
)

type Service struct {
	Store *tasks.Store
}

type Reply struct {
	Message string `json:"message"`
}

func (s *Service) Respond(ctx context.Context, userMessage string) (Reply, error) {
	if s.Store == nil {
		return Reply{}, fmt.Errorf("observer: store is required")
	}

	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return Reply{Message: "请先描述你的问题。"}, nil
	}

	lower := strings.ToLower(msg)
	all, err := s.Store.ListTasks(ctx, 500)
	if err != nil {
		return Reply{}, err
	}

	if looksLikeCountQuery(msg, lower) {
		running := 0
		for _, t := range all {
			if t.Status == tasks.StatusRunning {
				running++
			}
		}
		return Reply{Message: fmt.Sprintf("当前有 %d 个任务在执行（running）。", running)}, nil
	}

	if looksLikeProblemQuery(msg, lower) {
		if len(all) == 0 {
			return Reply{Message: "当前还没有任务。"}, nil
		}
		sort.SliceStable(all, func(i, j int) bool {
			if all[i].Score == all[j].Score {
				return all[i].CreatedAt.After(all[j].CreatedAt)
			}
			return all[i].Score > all[j].Score
		})
		top := all[0]
		return Reply{Message: fmt.Sprintf("我认为问题最多的是任务 %s（score=%d, status=%s, stderr=%d）。", top.ID, top.Score, top.Status, top.StderrCount)}, nil
	}

	if strings.Contains(lower, "system") || strings.Contains(msg, "系统") || strings.Contains(msg, "机器") {
		info := systeminfo.Snapshot()
		return Reply{Message: fmt.Sprintf("系统信息：%s %s，主机名 %s。", info.OS, info.Arch, info.Hostname)}, nil
	}

	return Reply{Message: "我可以回答：当前运行任务数量、最有问题的任务、以及服务器系统信息。你也可以直接问：'我们有几个任务在执行' / '哪个任务问题比较多'。"}, nil
}

func looksLikeCountQuery(msg, lower string) bool {
	return strings.Contains(msg, "几个任务") ||
		strings.Contains(msg, "多少任务") ||
		strings.Contains(msg, "在执行") ||
		strings.Contains(lower, "how many") ||
		strings.Contains(lower, "running tasks")
}

func looksLikeProblemQuery(msg, lower string) bool {
	return (strings.Contains(msg, "哪个任务") || strings.Contains(lower, "which task")) &&
		(strings.Contains(msg, "问题") || strings.Contains(lower, "problem") || strings.Contains(lower, "issues"))
}

