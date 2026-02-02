package observer

import (
	"context"
	"database/sql"
	"encoding/json"
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

	lower := strings.ToLower(msg)
	if looksLikeResumeQuery(msg, lower) {
		return s.handleResumeQuery(ctx, msg, lower)
	}

	backend := s.selectBackend(opts.Backend)
	if backend != nil {
		llmMsg := msg
		if s.Chat != nil {
			ctxText, err := s.recentChatContext(ctx, msg)
			if err == nil && strings.TrimSpace(ctxText) != "" {
				llmMsg = ctxText + "\n\nCurrent user message:\n" + msg
			}
		}
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

当你收到【Delivery Foreman / 交付前哨】一类的请求时（通常包含 run_id / session_id / recent_logs_tail）：
- 你 MUST 先判断该 run 是否真的“可交付完成”
- 你 MUST 判断“是否复杂任务”，并给出一句话理由（无硬阈值；不确定时 SHOULD 更保守）
- 若为复杂任务：你 MUST 进入【Acceptance Gates / 验收闸门】闭环：
  - 先调用 acceptance_prepare({task_id: run_id, max_iterations: 10}) 获取本轮 iteration，并确保不会超过上限
  - 若 acceptance_prepare 返回 can_continue=false：你 MUST 停止自动迭代并升级给用户（最小下一步 + 证据）
  - 先调用 acceptance_get({task_id: run_id}) 获取当前验收状态（用于避免重复触发/无限循环）
  - 你 SHOULD 调用 acceptance_build_contract({task_id: run_id}) 得到 deterministic baseline 的 plan_json，然后再结合用户要求补齐/修正
  - 使用 acceptance_update 写入/更新验收状态（status/iteration/current_gate/summary/plan_json/report），确保 UI 可见进度（iteration i/10）
  - 对“客观标准（Objective）”优先用确定性证据：调用 acceptance_evaluate_objectives({task_id: run_id, plan_json}) 或 task_output_stats 统计 words/sections/字数，并把测量值写入报告
  - 对“主观标准（Subjective）”必须先拆解 rubric，再逐项判定 pass/fail 并给出修改建议
  - 若验收不通过且仍可自动推进：你 SHOULD 调用 task_resume 创建新的 resume run（把失败项变成下一轮最小修复动作，并在回复里包含新 run id + 本轮目标）
  - 若达到迭代上限（默认 10）或遇到高风险/信息不足：你 MUST 停止自动迭代并升级给用户（最小下一步 + 证据）

你必须只输出一种结构化格式（不要输出 Markdown / 代码块 / 解释文字）。优先使用 tag 格式（长文本更稳，不需要 JSON 转义）：

1) 调用工具：
<action>tool</action>
<tool><tool_name></tool>
<args>{...json...}</args>

2) 最终回答：
<action>final</action>
<message><中文回答></message>

注意：不要在 <message> 内包含字面 "<action>" 或 "<message>"。

同时兼容 legacy JSON 格式：
{"action":"tool","tool":"<tool_name>","args":{...}}
{"action":"final","message":"<中文回答>"}`,
			OnToolCall:   opts.OnToolCall,
			OnToolResult: opts.OnToolResult,
		}

		ans, err := agent.Run(ctx, llmMsg)
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

	if looksLikeDeliveryForemanPrompt(msg, lower) {
		return s.handleDeliveryForemanFallback(ctx, msg)
	}

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

var reRunIDLine = regexp.MustCompile(`(?m)^\s*run_id:\s*([0-9a-fA-F-]{8,64})\s*$`)

func looksLikeDeliveryForemanPrompt(msg string, lower string) bool {
	if strings.Contains(lower, "delivery foreman") || strings.Contains(msg, "交付前哨") {
		return true
	}
	// Heuristic: our prompt format includes run_id + recent_logs_tail blocks.
	return strings.Contains(lower, "run_id:") && strings.Contains(lower, "runs_in_session:")
}

func extractRunIDFromMessage(msg string) string {
	m := reRunIDLine.FindStringSubmatch(msg)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func (s *Service) handleDeliveryForemanFallback(ctx context.Context, msg string) (Reply, error) {
	if s == nil || s.Store == nil {
		return Reply{}, fmt.Errorf("tasks store not configured")
	}

	runID := extractRunIDFromMessage(msg)
	if runID == "" {
		return Reply{Message: "Delivery Foreman（fallback）：无法解析 run_id。"}, nil
	}
	resolved, err := s.resolveTaskID(ctx, runID)
	if err != nil {
		return Reply{Message: fmt.Sprintf("Delivery Foreman（fallback）：找不到任务 %q。", runID)}, nil
	}
	t, err := s.Store.GetTask(ctx, resolved)
	if err != nil {
		return Reply{}, err
	}

	prompt := s.bestEffortSessionPrompt(ctx, t)
	complex, reason, signals := classifyComplexityHeuristic(prompt)
	if !complex {
		return Reply{Message: fmt.Sprintf("Delivery Foreman（fallback）：判断为简单任务（%s），无需验收闸门。", reason)}, nil
	}

	plan := buildAcceptancePlanHeuristic(prompt)
	plan.ComplexityReason = reason
	planJSON, _ := json.Marshal(plan)

	out, err := s.latestTaskOutput(ctx, t)
	if err != nil {
		return Reply{}, err
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

	var sb strings.Builder
	sb.WriteString("## Acceptance (deterministic fallback)\n\n")
	sb.WriteString("- LLM backend: **unavailable** (cannot run rubric evaluation / auto-iteration)\n")
	sb.WriteString(fmt.Sprintf("- run_id: `%s`\n", t.ID))
	if sid := strings.TrimSpace(t.SessionID); sid != "" {
		sb.WriteString(fmt.Sprintf("- session_id: `%s`\n", sid))
	}
	sb.WriteString(fmt.Sprintf("- complexity: **complex** — %s\n", reason))
	if len(signals) > 0 {
		sb.WriteString(fmt.Sprintf("- signals: %s\n", strings.Join(signals, ", ")))
	}

	sb.WriteString("\n### Objective\n")
	if len(results) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		for _, r := range results {
			status := "FAIL"
			if r.Pass {
				status = "PASS"
			}
			sb.WriteString(fmt.Sprintf("- %s %s (measured=%d %s, min=%d, max=%d)\n", status, r.Title, r.Measured, r.Unit, r.Min, r.Max))
		}
	}

	sb.WriteString("\n### Subjective\n")
	if len(plan.SubjectiveRubrics) == 0 {
		sb.WriteString("- (none)\n")
	} else {
		sb.WriteString("- (requires LLM) subjective rubrics present but not evaluated.\n")
	}

	sb.WriteString("\n### Next\n")
	sb.WriteString("- 配置一个可用的 LLM backend（claude/codex），以启用主观 rubric 验收与自动迭代。\n")
	sb.WriteString("- 或者手动执行默认验证步骤，并把证据写入日志供验收。\n")

	key := tasks.SessionKeyForTask(t)
	prev, ok, err := s.Store.GetAcceptanceState(ctx, key)
	if err != nil {
		return Reply{}, err
	}
	iter := 1
	maxIter := 10
	if ok {
		if prev.Iteration > 0 {
			iter = prev.Iteration
		}
		if prev.MaxIterations > 0 {
			maxIter = prev.MaxIterations
		}
	}
	_, _ = s.Store.UpsertAcceptanceState(ctx, tasks.UpsertAcceptanceStateInput{
		Key:           key,
		Status:        "failed",
		Iteration:     iter,
		MaxIterations: maxIter,
		CurrentGate:   "fallback",
		Summary:       "LLM backend unavailable; objective-only evaluation",
		PlanJSON:      string(planJSON),
		Report:        sb.String(),
		RunID:         t.ID,
	})

	return Reply{Message: "Delivery Foreman（fallback）：已生成验收报告（仅客观标准；主观标准需 LLM 才能拆解与判断）。"}, nil
}

func (s *Service) recentChatContext(ctx context.Context, currentUserMessage string) (string, error) {
	if s == nil || s.Chat == nil {
		return "", nil
	}

	msgs, err := s.Chat.Tail(ctx, 12)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "", nil
	}

	// When called via /api/chat, the current user message has already been appended.
	last := msgs[len(msgs)-1]
	if last.Role == chat.RoleUser && strings.TrimSpace(last.Content) == strings.TrimSpace(currentUserMessage) {
		msgs = msgs[:len(msgs)-1]
	}
	if len(msgs) == 0 {
		return "", nil
	}

	const maxPerMessage = 800
	var sb strings.Builder
	sb.WriteString("Recent chat context:\n")
	for _, m := range msgs {
		role := strings.ToUpper(string(m.Role))
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\r", "\n")
		content = strings.ReplaceAll(content, "\n", "\\n")
		content = truncateRunes(content, maxPerMessage)
		sb.WriteString(role)
		sb.WriteString(": ")
		sb.WriteString(content)
		sb.WriteByte('\n')
	}
	out := strings.TrimSpace(sb.String())
	if out == "Recent chat context:" {
		return "", nil
	}
	return out, nil
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var sb strings.Builder
	// Best-effort sizing. UTF-8 can be up to 4 bytes per rune.
	sb.Grow(max * 4)
	n := 0
	for _, r := range s {
		if n >= max-1 {
			break
		}
		sb.WriteRune(r)
		n++
	}
	sb.WriteRune('…')
	return sb.String()
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

func looksLikeResumeQuery(msg, lower string) bool {
	// Be conservative: only treat explicit commands as "resume".
	// Avoid triggering on casual mentions like "只会回答continue".
	s := strings.TrimSpace(msg)
	if s == "" {
		return false
	}

	s = stripResumePreface(s)
	lower = strings.ToLower(s)

	if startsWithEnglishResumeCommand(lower, "continue") ||
		startsWithEnglishResumeCommand(lower, "resume") ||
		startsWithEnglishResumeCommand(lower, "retry") {
		return true
	}

	if startsWithChineseResumeCommand(s, "继续") ||
		startsWithChineseResumeCommand(s, "恢复") ||
		startsWithChineseResumeCommand(s, "重试") {
		return true
	}

	return false
}

func stripResumePreface(s string) string {
	s = strings.TrimSpace(s)
	// Keep this list small and deterministic; it is only for intent parsing.
	prefixes := []string{
		"请帮我",
		"请帮忙",
		"麻烦你",
		"麻烦您",
		"麻烦",
		"帮我",
		"请",
	}

	for i := 0; i < 2; i++ {
		trimmed := strings.TrimSpace(s)
		changed := false
		for _, p := range prefixes {
			if strings.HasPrefix(trimmed, p) {
				s = strings.TrimSpace(strings.TrimPrefix(trimmed, p))
				s = strings.TrimLeft(s, " :：,，\t")
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(s)
}

func startsWithEnglishResumeCommand(lower string, keyword string) bool {
	if !strings.HasPrefix(lower, keyword) {
		return false
	}
	if len(lower) == len(keyword) {
		return true
	}
	next := lower[len(keyword)]
	if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') || next == '_' {
		// Not a word boundary.
		return false
	}

	rest := strings.TrimSpace(lower[len(keyword):])
	if rest == "" {
		return true
	}
	return resumeHasTargetHint(rest)
}

func startsWithChineseResumeCommand(s string, keyword string) bool {
	if !strings.HasPrefix(s, keyword) {
		return false
	}

	rest := s[len(keyword):]
	if rest == "" {
		return true
	}

	r, _ := utf8.DecodeRuneInString(rest)
	if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
		return true
	}
	if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
		return true
	}

	// Common short command particles.
	for _, suf := range []string{"一下", "下", "吧", "呀", "呢", "吗", "么", "啊", "哈"} {
		if !strings.HasPrefix(rest, suf) {
			continue
		}
		after := rest[len(suf):]
		if strings.TrimSpace(after) == "" {
			return true
		}
		rr, _ := utf8.DecodeRuneInString(after)
		if unicode.IsSpace(rr) || unicode.IsPunct(rr) || unicode.IsSymbol(rr) {
			return true
		}
		if rr <= unicode.MaxASCII && (unicode.IsLetter(rr) || unicode.IsDigit(rr)) {
			return true
		}
		return resumeHasTargetHint(after)
	}

	// If the remaining text references a task/session explicitly, treat it as a resume command.
	return resumeHasTargetHint(rest)
}

func resumeHasTargetHint(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	// IDs/prefixes strongly suggest intent.
	if reIDPrefix.FindString(strings.ToLower(s)) != "" {
		return true
	}
	lower := strings.ToLower(s)
	return strings.Contains(s, "任务") ||
		strings.Contains(s, "会话") ||
		strings.Contains(lower, "task") ||
		strings.Contains(lower, "run") ||
		strings.Contains(lower, "session") ||
		strings.Contains(lower, "sess")
}

func pickResumeTargetByPrefix(all []tasks.Task, prefix string) (tasks.Task, bool) {
	p := strings.ToLower(strings.TrimSpace(prefix))
	if p == "" {
		return tasks.Task{}, false
	}
	var best tasks.Task
	found := false
	for _, t := range all {
		id := strings.ToLower(t.ID)
		sid := strings.ToLower(strings.TrimSpace(t.SessionID))
		if strings.HasPrefix(id, p) || (sid != "" && strings.HasPrefix(sid, p)) {
			if !found || t.UpdatedAt.After(best.UpdatedAt) {
				best = t
				found = true
			}
		}
	}
	return best, found
}

func pickResumeTargetAuto(all []tasks.Task) (tasks.Task, bool) {
	if len(all) == 0 {
		return tasks.Task{}, false
	}

	runningBySession := map[string]bool{}
	for _, t := range all {
		sid := strings.TrimSpace(t.SessionID)
		if sid == "" {
			continue
		}
		if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued {
			runningBySession[sid] = true
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})

	for _, t := range all {
		if t.Status != tasks.StatusFailed && t.Status != tasks.StatusBlocked && t.Status != tasks.StatusInterrupted {
			continue
		}
		if t.SessionDeletedAt != nil {
			continue
		}
		sid := strings.TrimSpace(t.SessionID)
		if sid == "" {
			continue
		}
		if runningBySession[sid] {
			continue
		}
		return t, true
	}
	return tasks.Task{}, false
}

func (s *Service) handleResumeQuery(ctx context.Context, msg string, lower string) (Reply, error) {
	if s.Store == nil {
		return Reply{}, fmt.Errorf("observer: store is required")
	}
	if s.Runner == nil {
		return Reply{Message: "当前无法继续：task runner 未配置。"}, nil
	}

	all, err := s.Store.ListTasks(ctx, 500)
	if err != nil {
		return Reply{}, err
	}
	if len(all) == 0 {
		return Reply{Message: "当前还没有任务。"}, nil
	}

	var target tasks.Task
	ok := false

	// Prefer explicit id/session references (8-hex prefix is enough for our UI).
	prefixes := reIDPrefix.FindAllString(lower, -1)
	for _, p := range prefixes {
		if t, found := pickResumeTargetByPrefix(all, p); found {
			target = t
			ok = true
			break
		}
	}

	if !ok {
		target, ok = pickResumeTargetAuto(all)
	}
	if !ok {
		return Reply{Message: "没有找到可继续的任务（failed/blocked/interrupted 且可 resume 的 session）。"}, nil
	}

	tool := s.agentTools()["task_resume"]
	if tool == nil {
		return Reply{Message: "当前无法继续：task_resume 不可用。"}, nil
	}

	res, err := tool.Run(ctx, map[string]any{"task_id": target.ID})
	if err != nil {
		return Reply{Message: fmt.Sprintf("无法继续：%v", err)}, nil
	}

	// Best-effort extract details for a human-friendly reply.
	newID := ""
	newSID := strings.TrimSpace(target.SessionID)
	newWorkdir := strings.TrimSpace(target.WorkDir)
	if m, ok := res.(map[string]any); ok {
		if tm, ok := m["task"].(map[string]any); ok {
			if v, ok := tm["id"].(string); ok {
				newID = strings.TrimSpace(v)
			}
			if v, ok := tm["session_id"].(string); ok && strings.TrimSpace(v) != "" {
				newSID = strings.TrimSpace(v)
			}
			if v, ok := tm["workdir"].(string); ok && strings.TrimSpace(v) != "" {
				newWorkdir = strings.TrimSpace(v)
			}
		}
	}

	idShort := newID
	if len(idShort) > 8 {
		idShort = idShort[:8]
	}
	sidShort := newSID
	if len(sidShort) > 8 {
		sidShort = sidShort[:8]
	}

	msgParts := []string{fmt.Sprintf("已创建新的 resume run：%s", idShort)}
	if strings.TrimSpace(sidShort) != "" {
		msgParts = append(msgParts, fmt.Sprintf("session %s", sidShort))
	}
	if strings.TrimSpace(newWorkdir) != "" {
		msgParts = append(msgParts, fmt.Sprintf("workdir %s", newWorkdir))
	}
	return Reply{Message: strings.Join(msgParts, " · ")}, nil
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

type headingStat struct {
	HeadingLines    int
	MarkdownHeading int
	NumberedHeading int
	ChineseHeading  int
}

func computeHeadingStat(s string) headingStat {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")

	var st headingStat
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if isMarkdownHeadingLine(line) {
			st.HeadingLines++
			st.MarkdownHeading++
			continue
		}
		if isNumberedHeadingLine(line) {
			st.HeadingLines++
			st.NumberedHeading++
			continue
		}
		if isChineseHeadingLine(line) {
			st.HeadingLines++
			st.ChineseHeading++
			continue
		}
	}
	return st
}

func isMarkdownHeadingLine(line string) bool {
	if !strings.HasPrefix(line, "#") {
		return false
	}
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 || i >= len(line) {
		return false
	}
	if line[i] != ' ' && line[i] != '\t' {
		return false
	}
	return strings.TrimSpace(line[i:]) != ""
}

func isNumberedHeadingLine(line string) bool {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(line) {
		return false
	}

	hasDotSegment := false
	for i < len(line) && line[i] == '.' {
		j := i + 1
		k := j
		for k < len(line) && line[k] >= '0' && line[k] <= '9' {
			k++
		}
		if k == j {
			break
		}
		hasDotSegment = true
		i = k
	}

	if i >= len(line) {
		return false
	}

	punc := false
	switch line[i] {
	case '.', ')':
		punc = true
		i++
	}

	if i >= len(line) {
		return false
	}
	if !punc && !hasDotSegment {
		return false
	}
	if line[i] != ' ' && line[i] != '\t' {
		return false
	}
	return strings.TrimSpace(line[i:]) != ""
}

func isChineseHeadingLine(line string) bool {
	r := []rune(line)
	if len(r) < 2 {
		return false
	}

	// Pattern: "第...章/节/篇/部/条"
	if r[0] == '第' {
		for i := 1; i < len(r); i++ {
			switch r[i] {
			case '章', '节', '篇', '部', '条':
				return i > 1
			}
		}
		return false
	}

	// Pattern: "一、标题"
	if len(r) >= 3 && r[1] == '、' {
		switch r[0] {
		case '一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '百', '千':
			return true
		}
	}
	return false
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
