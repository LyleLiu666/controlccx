package observer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"controlccx/internal/chat"
	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
)

type Service struct {
	Store  *tasks.Store
	Chat   *chat.Store
	Runner TaskRunner
	LLM    Backend
	Claude Backend
	Codex  Backend

	// AgentMaxSteps limits the tool-call loop iterations. If zero, a default is used.
	AgentMaxSteps int

	// ForceAgent disables heuristic fallback when an LLM backend is configured.
	ForceAgent bool
}

type RespondOptions struct {
	Backend  string // "auto" | "claude" | "codex"
	MaxSteps int

	OnToolCall   func(tool string, args map[string]any)
	OnToolResult func(tool string, result any)
}

type TaskRunner interface {
	Start(ctx context.Context, taskID string) error
	Cancel(taskID string) bool
}

type Reply struct {
	Message string `json:"message"`
}

func (s *Service) Respond(ctx context.Context, userMessage string) (Reply, error) {
	return s.RespondWithOptions(ctx, userMessage, RespondOptions{})
}

func (s *Service) RespondWithOptions(ctx context.Context, userMessage string, opts RespondOptions) (Reply, error) {
	if s.Store == nil {
		return Reply{}, fmt.Errorf("observer: store is required")
	}

	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return Reply{Message: "请先描述你的问题。"}, nil
	}

	backend := s.selectBackend(opts.Backend)
	if backend != nil {
		agent := Agent{
			LLM:      backend,
			Tools:    s.agentTools(),
			MaxSteps: pickMaxSteps(opts.MaxSteps, s.AgentMaxSteps),
			SystemPrompt: `你是 ControlCCX 的秘书（一个具备 tool 调用能力的 agent）。

你必须优先使用提供的 tools 获取信息来回答问题，不能编造任务/日志/系统信息。

当需要引用某个任务时：
- 优先用 tasks_list 查找候选任务，并使用返回的 id
- 如果用户只给了“任务名称/关键词”，你也可以把该关键词直接作为 task_id 传入（系统会用 prompt/session_id/id 前缀进行匹配）；若匹配多个，请先向用户确认

当用户要求“继续/恢复/重试”某个已中断/失败/阻塞的任务时：
- 你 SHOULD 尽量直接调用 task_resume / task_cancel 等操作工具来帮用户推进（除非你需要用户确认某个高风险选择）
- 你 MUST 在最终回复里说明你做了什么（例如：已创建新的 resume run / 已取消任务 / 为什么没法继续）

你必须只输出一个 JSON 对象（不要输出 Markdown / 代码块 / 解释文字）。

可用输出格式：
1) 调用工具：
{"action":"tool","tool":"<tool_name>","args":{...}}
2) 最终回答：
{"action":"final","message":"<中文回答>"}`,
			OnToolCall:   opts.OnToolCall,
			OnToolResult: opts.OnToolResult,
		}

		ans, err := agent.Run(ctx, msg)
		if err == nil && strings.TrimSpace(ans) != "" {
			return Reply{Message: ans}, nil
		}
		if s.ForceAgent {
			if err == nil {
				err = fmt.Errorf("llm agent returned empty response")
			}
			return Reply{}, err
		}
		// Fall back to deterministic heuristics if the agent is unavailable/fails.
	}

	if s.ForceAgent && strings.TrimSpace(opts.Backend) != "" && backend == nil {
		return Reply{}, fmt.Errorf("llm backend not available for backend=%q", strings.TrimSpace(opts.Backend))
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

	if looksLikeLengthQuery(msg, lower) {
		return s.answerLengthQuery(ctx, msg, all)
	}

	if strings.Contains(lower, "system") || strings.Contains(msg, "系统") || strings.Contains(msg, "机器") {
		info := systeminfo.Snapshot()
		return Reply{Message: fmt.Sprintf("系统信息：%s %s，主机名 %s。", info.OS, info.Arch, info.Hostname)}, nil
	}

	return Reply{Message: "我可以回答：当前运行任务数量、最有问题的任务、任务结果字数是否达标、以及服务器系统信息。你也可以直接问：'我们有几个任务在执行' / '哪个任务问题比较多' / '刚刚那个写脱口秀的任务字数够不够'。"}, nil
}

func pickMaxSteps(requested int, fallback int) int {
	if requested > 0 && requested <= 32 {
		return requested
	}
	if fallback > 0 {
		return fallback
	}
	return 8
}

func (s *Service) selectBackend(name string) Backend {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "auto":
		return s.LLM
	case "claude":
		if s.Claude != nil {
			return s.Claude
		}
		return s.LLM
	case "codex":
		if s.Codex != nil {
			return s.Codex
		}
		return s.LLM
	default:
		return s.LLM
	}
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

func looksLikeLengthQuery(msg, lower string) bool {
	return strings.Contains(msg, "字数") ||
		strings.Contains(msg, "多少字") ||
		strings.Contains(msg, "几字") ||
		strings.Contains(lower, "word count") ||
		(strings.Contains(lower, "words") && (strings.Contains(lower, "enough") || strings.Contains(lower, "count")))
}

type lengthReq struct {
	Min  int
	Unit string // "字" | "words"
}

var (
	reLenReq   = regexp.MustCompile(`(?i)(\d{1,6})\s*(字|个字|字符|words?)`)
	reIDPrefix = regexp.MustCompile(`(?i)\b[0-9a-f]{8}\b`)
)

func parseLengthReq(text string) (lengthReq, bool) {
	m := reLenReq.FindStringSubmatch(text)
	if len(m) < 3 {
		return lengthReq{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return lengthReq{}, false
	}
	unitRaw := strings.ToLower(strings.TrimSpace(m[2]))
	unit := "字"
	if strings.Contains(unitRaw, "word") {
		unit = "words"
	}
	return lengthReq{Min: n, Unit: unit}, true
}

func pickTaskForMessage(msg string, all []tasks.Task) (tasks.Task, bool) {
	if len(all) == 0 {
		return tasks.Task{}, false
	}

	lower := strings.ToLower(msg)
	if prefix := strings.ToLower(reIDPrefix.FindString(lower)); prefix != "" {
		for i := range all {
			id := strings.ToLower(all[i].ID)
			sid := strings.ToLower(strings.TrimSpace(all[i].SessionID))
			if strings.HasPrefix(id, prefix) || (sid != "" && strings.HasPrefix(sid, prefix)) {
				return all[i], true
			}
		}
	}

	if strings.Contains(msg, "脱口秀") || strings.Contains(lower, "standup") || strings.Contains(lower, "comedy") {
		for i := range all {
			p := all[i].Prompt
			pl := strings.ToLower(p)
			if strings.Contains(p, "脱口秀") || strings.Contains(pl, "standup") || strings.Contains(pl, "comedy") {
				return all[i], true
			}
		}
	}

	return all[0], true
}

type lengthStat struct {
	NonSpaceRunes int
	Runes         int
	Words         int
}

func computeLengthStat(s string) lengthStat {
	nonSpace := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		nonSpace++
	}
	return lengthStat{
		NonSpaceRunes: nonSpace,
		Runes:         utf8.RuneCountInString(s),
		Words:         len(strings.Fields(s)),
	}
}

func (s *Service) answerLengthQuery(ctx context.Context, msg string, all []tasks.Task) (Reply, error) {
	task, ok := pickTaskForMessage(msg, all)
	if !ok {
		return Reply{Message: "当前还没有任务。"}, nil
	}

	out, err := s.latestTaskOutput(ctx, task)
	if err != nil {
		return Reply{}, err
	}
	if strings.TrimSpace(out) == "" {
		return Reply{Message: fmt.Sprintf("任务 %s 还没有可统计的输出（status=%s）。", task.ID[:8], task.Status)}, nil
	}

	stat := computeLengthStat(out)

	req, ok := parseLengthReq(msg)
	if !ok {
		req, ok = parseLengthReq(task.Prompt)
	}

	if ok && req.Min > 0 {
		enough := false
		switch req.Unit {
		case "words":
			enough = stat.Words >= req.Min
			if enough {
				return Reply{Message: fmt.Sprintf("任务 %s 输出约 %d words。要求 >= %d words：够。", task.ID[:8], stat.Words, req.Min)}, nil
			}
			return Reply{Message: fmt.Sprintf("任务 %s 输出约 %d words。要求 >= %d words：不够。", task.ID[:8], stat.Words, req.Min)}, nil
		default:
			enough = stat.NonSpaceRunes >= req.Min
			if enough {
				return Reply{Message: fmt.Sprintf("任务 %s 输出约 %d 字（去空白）。要求 >= %d 字：够。", task.ID[:8], stat.NonSpaceRunes, req.Min)}, nil
			}
			return Reply{Message: fmt.Sprintf("任务 %s 输出约 %d 字（去空白）。要求 >= %d 字：不够。", task.ID[:8], stat.NonSpaceRunes, req.Min)}, nil
		}
	}

	return Reply{Message: fmt.Sprintf("任务 %s 输出约 %d 字（去空白）。你期望至少多少字？（例如：>=800字）", task.ID[:8], stat.NonSpaceRunes)}, nil
}

func (s *Service) latestTaskOutput(ctx context.Context, t tasks.Task) (string, error) {
	switch t.WorkerType {
	case tasks.WorkerClaudeCode, tasks.WorkerCodex:
		e, err := s.Store.LatestLog(ctx, t.ID, tasks.LogAssistant)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			return "", err
		}
		return e.Message, nil
	case tasks.WorkerExec:
		// exec has no assistant stream; approximate by concatenating stdout lines.
		logs, err := s.Store.ListLogs(ctx, t.ID, 0, 2000)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for _, e := range logs {
			if e.Stream != tasks.LogStdout {
				continue
			}
			sb.WriteString(e.Message)
			sb.WriteByte('\n')
		}
		return strings.TrimSpace(sb.String()), nil
	default:
		e, err := s.Store.LatestLog(ctx, t.ID, tasks.LogAssistant)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			return "", err
		}
		return e.Message, nil
	}
}
