package observer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type AcceptancePlan struct {
	IntentSummary     string                         `json:"intent_summary,omitempty"`
	ComplexityReason  string                         `json:"complexity_reason,omitempty"`
	ObjectiveCriteria []AcceptanceObjectiveCriterion `json:"objective_criteria,omitempty"`
	SubjectiveRubrics []AcceptanceSubjectiveRubric   `json:"subjective_rubrics,omitempty"`
	DefaultGates      []AcceptanceDefaultGate        `json:"default_gates,omitempty"`
}

type AcceptanceObjectiveCriterion struct {
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Method string `json:"method,omitempty"`
	Min    int    `json:"min,omitempty"`
	Max    int    `json:"max,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

type AcceptanceSubjectiveRubric struct {
	ID    string                 `json:"id,omitempty"`
	Title string                 `json:"title,omitempty"`
	Items []AcceptanceRubricItem `json:"items,omitempty"`
}

type AcceptanceRubricItem struct {
	Item         string `json:"item,omitempty"`
	PassCriteria string `json:"pass_criteria,omitempty"`
}

type AcceptanceDefaultGate struct {
	ID        string   `json:"id,omitempty"`
	AppliesIf string   `json:"applies_if,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Steps     []string `json:"steps,omitempty"`
	Fallback  string   `json:"fallback,omitempty"`
}

type AcceptanceEvidenceRef struct {
	Kind string `json:"kind,omitempty"`
	Ref  string `json:"ref,omitempty"`
	Note string `json:"note,omitempty"`
}

type AcceptanceObjectiveResult struct {
	ID       string                  `json:"id,omitempty"`
	Title    string                  `json:"title,omitempty"`
	Method   string                  `json:"method,omitempty"`
	Pass     bool                    `json:"pass"`
	Measured int                     `json:"measured,omitempty"`
	Min      int                     `json:"min,omitempty"`
	Max      int                     `json:"max,omitempty"`
	Unit     string                  `json:"unit,omitempty"`
	Evidence []AcceptanceEvidenceRef `json:"evidence,omitempty"`
	Note     string                  `json:"note,omitempty"`
}

var (
	reSectionsReq = regexp.MustCompile(`(?i)(\d{1,4})\s*(个部分|部分|章节|章|段落|parts?|sections?)`)
)

func parseSectionsReq(text string) (int, bool) {
	m := reSectionsReq.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func classifyComplexityHeuristic(prompt string) (bool, string, []string) {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return false, "没有明确交付物信息", nil
	}
	pl := strings.ToLower(p)

	signals := make([]string, 0, 8)
	score := 0

	// High-signal runnable/product phrases.
	high := []string{
		"从0", "从 0", "0开发", "0 开发",
		"全栈", "前后端", "数据库", "后端", "前端",
		"project", "full-stack", "backend", "frontend", "database",
		"run起来", "能跑起来", "可运行", "一键启动", "smoke", "playwright",
		"skills hub", "marketplace", "dockerhub", "github",
	}
	for _, kw := range high {
		if strings.Contains(pl, strings.ToLower(kw)) || strings.Contains(p, kw) {
			signals = append(signals, kw)
			score += 3
		}
	}

	// Multi-step / quality gates hints.
	medium := []string{
		"测试", "验收", "部署", "上线", "架构", "重构", "性能", "安全",
		"go test", "pnpm test", "npm test", "pytest", "unit test",
	}
	for _, kw := range medium {
		if strings.Contains(pl, strings.ToLower(kw)) || strings.Contains(p, kw) {
			signals = append(signals, kw)
			score++
		}
	}

	// Hard objective constraints also imply complexity.
	if req, ok := parseLengthReq(p); ok && req.Min > 0 {
		signals = append(signals, fmt.Sprintf("len=%d%s", req.Min, req.Unit))
		if req.Min >= 5000 {
			score += 2
		} else {
			score++
		}
	}
	if n, ok := parseSectionsReq(p); ok {
		signals = append(signals, fmt.Sprintf("sections=%d", n))
		score += 2
	}

	if len([]rune(p)) >= 240 {
		signals = append(signals, "long_prompt")
		score++
	}

	complex := score >= 3
	if complex {
		reason := "包含可运行/多步骤交付或硬约束，失败风险高，建议启用验收闸门"
		return true, reason, dedupeStrings(signals)
	}
	return false, "任务看起来较简单，验收闸门可能是噪音", dedupeStrings(signals)
}

func buildAcceptancePlanHeuristic(prompt string) AcceptancePlan {
	p := strings.TrimSpace(prompt)
	complex, reason, _ := classifyComplexityHeuristic(p)

	intent := "deliverable"
	pl := strings.ToLower(p)
	if strings.Contains(p, "文章") || strings.Contains(p, "写") || strings.Contains(pl, "article") || strings.Contains(pl, "report") {
		intent = "writing deliverable"
	}
	if strings.Contains(p, "项目") || strings.Contains(p, "开发") || strings.Contains(pl, "project") || strings.Contains(pl, "full-stack") {
		intent = "runnable deliverable"
	}

	var objectives []AcceptanceObjectiveCriterion
	if req, ok := parseLengthReq(p); ok && req.Min > 0 {
		if req.Unit == "words" {
			objectives = append(objectives, AcceptanceObjectiveCriterion{
				ID:     "word_count",
				Title:  fmt.Sprintf(">=%d words", req.Min),
				Method: "task_output_stats.words",
				Min:    req.Min,
				Unit:   "words",
			})
		} else {
			objectives = append(objectives, AcceptanceObjectiveCriterion{
				ID:     "char_count",
				Title:  fmt.Sprintf(">=%d chars(no-space)", req.Min),
				Method: "task_output_stats.chars_no_space",
				Min:    req.Min,
				Unit:   "chars_no_space",
			})
		}
	}
	if n, ok := parseSectionsReq(p); ok && n > 0 {
		objectives = append(objectives, AcceptanceObjectiveCriterion{
			ID:     "sections",
			Title:  fmt.Sprintf(">=%d sections", n),
			Method: "task_output_stats.sections",
			Min:    n,
			Unit:   "sections",
		})
	}

	var rubrics []AcceptanceSubjectiveRubric
	if strings.Contains(p, "公众号") || strings.Contains(pl, "wechat") {
		rubrics = append(rubrics, AcceptanceSubjectiveRubric{
			ID:    "wechat",
			Title: "适合公众号",
			Items: []AcceptanceRubricItem{
				{Item: "标题/导语吸引力", PassCriteria: "标题清晰具体，有利益点或冲突点；导语能快速说明读者收益"},
				{Item: "结构与小标题", PassCriteria: "有明确分段与小标题；先结论后展开；逻辑顺序清晰"},
				{Item: "可读性", PassCriteria: "句子短、术语解释到位；段落不拥挤；关键信息有强调"},
				{Item: "信息密度与证据", PassCriteria: "关键信息有数据/出处；不过度堆砌；能被读者快速抓住要点"},
				{Item: "行动指引/CTA", PassCriteria: "结尾给出下一步建议或讨论问题（不夸大承诺）"},
				{Item: "合规与边界提示", PassCriteria: "对医疗/用药类内容给出必要免责声明与边界说明"},
			},
		})
	}

	var gates []AcceptanceDefaultGate
	if complex && (strings.Contains(p, "项目") || strings.Contains(pl, "project") || strings.Contains(p, "可运行") || strings.Contains(p, "run")) {
		gates = append(gates, AcceptanceDefaultGate{
			ID:        "runnable.dod",
			AppliesIf: "deliverable must run",
			Reason:    "降低“跑不起来/没测试/不可交付”的高概率失败",
			Steps: []string{
				"检查 README 是否包含一键启动/Quick Start（给出单行命令）",
				"若存在 go.mod：运行 `go test ./...`",
				"若存在 package.json 且包含 test script：运行 `pnpm test`（或项目约定的 test 命令）",
				"启动服务并做 HTTP smoke（访问首页/health，返回 200/可用页面），然后干净退出",
			},
			Fallback: "若 Playwright MCP 不可用，降级为 HTTP smoke，并在报告里记录原因",
		})
	}

	return AcceptancePlan{
		IntentSummary:     intent,
		ComplexityReason:  reason,
		ObjectiveCriteria: objectives,
		SubjectiveRubrics: rubrics,
		DefaultGates:      gates,
	}
}

func parseAcceptancePlanJSON(planJSON string) (AcceptancePlan, error) {
	planJSON = strings.TrimSpace(planJSON)
	if planJSON == "" {
		return AcceptancePlan{}, fmt.Errorf("acceptance plan is empty")
	}
	var plan AcceptancePlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return AcceptancePlan{}, fmt.Errorf("invalid acceptance plan json: %w", err)
	}
	return plan, nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	return out
}

func acceptanceMeasureForMethod(method string, st lengthStat, hs headingStat) (int, string, bool) {
	m := strings.ToLower(strings.TrimSpace(method))
	switch m {
	case "task_output_stats.words", "words", "word_count":
		return st.Words, "words", true
	case "task_output_stats.chars_no_space", "chars_no_space", "char_count":
		return st.NonSpaceRunes, "chars_no_space", true
	case "task_output_stats.sections", "task_output_stats.heading_lines", "sections", "heading_lines":
		return hs.HeadingLines, "sections", true
	default:
		return 0, "", false
	}
}
