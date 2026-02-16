package secretary

import (
	"strings"
	"testing"

	sectools "controlccx/internal/secretary/tools"
)

func extractToolSection(t *testing.T, prompt string) string {
	t.Helper()
	start := strings.Index(prompt, "工具说明：")
	if start < 0 {
		t.Fatalf("prompt missing tools header")
	}
	start += len("工具说明：")

	endRel := strings.Index(prompt[start:], "\n如果用户问")
	if endRel < 0 {
		t.Fatalf("prompt missing tools footer")
	}
	section := prompt[start : start+endRel]
	return strings.Trim(section, "\n")
}

func parseToolLines(t *testing.T, toolSection string) map[string]string {
	t.Helper()
	lines := strings.Split(toolSection, "\n")
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rest := strings.TrimPrefix(line, "- ")
		iParen := strings.Index(rest, "(")
		iColon := strings.Index(rest, "：")
		end := -1
		switch {
		case iParen >= 0 && iColon >= 0:
			if iParen < iColon {
				end = iParen
			} else {
				end = iColon
			}
		case iParen >= 0:
			end = iParen
		case iColon >= 0:
			end = iColon
		default:
			t.Fatalf("invalid tool line: %q", line)
		}
		name := strings.TrimSpace(rest[:end])
		if name == "" {
			t.Fatalf("invalid tool line (empty name): %q", line)
		}
		if _, exists := out[name]; exists {
			t.Fatalf("duplicate tool line: %q", name)
		}
		out[name] = line
	}
	return out
}

func findRunOptsGroupLine(t *testing.T, toolSection string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(toolSection, "\n") {
		if strings.HasPrefix(line, "安全参数组 [runopts]：") {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 runopts group line, found %d", len(found))
	}
	return found[0]
}

func TestBuildSystemPrompt_ContainsAllToolDescriptorsOnce(t *testing.T) {
	prompt := buildSystemPrompt()
	toolSection := extractToolSection(t, prompt)
	toolLines := parseToolLines(t, toolSection)

	descriptors := sectools.Descriptors()
	if len(toolLines) != len(descriptors) {
		t.Fatalf("unexpected tool line count: got %d want %d", len(toolLines), len(descriptors))
	}

	for _, d := range descriptors {
		name := strings.TrimSpace(d.Name)
		desc := strings.TrimSpace(d.DescriptionZH)
		if name == "" {
			t.Fatalf("found empty tool name")
		}
		if desc == "" {
			t.Fatalf("tool %q has empty Chinese description", name)
		}
		line, ok := toolLines[name]
		if !ok {
			t.Fatalf("prompt missing tool line: %s", name)
		}
		if !strings.Contains(line, "："+desc) {
			t.Fatalf("prompt tool line missing description for %q: %q", name, line)
		}
	}
}

func TestBuildSystemPrompt_ToolParamsRunOptsAreStructured(t *testing.T) {
	prompt := buildSystemPrompt()
	toolSection := extractToolSection(t, prompt)
	toolLines := parseToolLines(t, toolSection)

	groupLine := findRunOptsGroupLine(t, toolSection)
	for _, key := range sectools.RunOptsParams {
		if strings.Count(toolSection, key) != 1 {
			t.Fatalf("expected runopts key %q appear exactly once in tool section", key)
		}
		if !strings.Contains(groupLine, key) {
			t.Fatalf("runopts group line missing key %q: %q", key, groupLine)
		}
	}

	runOptsTools := []string{
		"task_continue_submit",
		"task_preempt_continue_submit",
		"task_resume_submit",
		"task_rehydrate_submit",
		"task_new_submit",
	}
	for _, name := range runOptsTools {
		line, ok := toolLines[name]
		if !ok {
			t.Fatalf("prompt missing tool line: %s", name)
		}
		if !strings.Contains(line, name+"(") {
			t.Fatalf("expected tool line to include params for %q: %q", name, line)
		}
		if !strings.Contains(line, "[runopts]") {
			t.Fatalf("expected tool line to include [runopts] for %q: %q", name, line)
		}
		for _, key := range sectools.RunOptsParams {
			if strings.Contains(line, key) {
				t.Fatalf("expected tool line %q to collapse runopts key %q: %q", name, key, line)
			}
		}
		if !strings.Contains(line, "必填：") {
			t.Fatalf("expected tool line to include required info for %q: %q", name, line)
		}
	}

	continueLine := toolLines["task_continue_submit"]
	if !strings.Contains(continueLine, "必填：task_id。") {
		t.Fatalf("expected task_continue_submit required rendering, got: %q", continueLine)
	}

	schedulerLine, ok := toolLines["scheduler_create"]
	if !ok {
		t.Fatalf("prompt missing tool line: scheduler_create")
	}
	if !strings.Contains(schedulerLine, "scheduler_create(") {
		t.Fatalf("expected scheduler_create to include params: %q", schedulerLine)
	}
	if !strings.Contains(schedulerLine, "必填：tool_fields_json。") {
		t.Fatalf("expected scheduler_create required rendering, got: %q", schedulerLine)
	}
	wantAnyOf := "必填（任选一组）：tool_name | target_tool_name | name。"
	if !strings.Contains(schedulerLine, wantAnyOf) {
		t.Fatalf("expected scheduler_create any-of rendering %q, got: %q", wantAnyOf, schedulerLine)
	}

	if strings.Count(toolSection, "安全参数组 [runopts]：") != 1 {
		t.Fatalf("runopts group line should appear exactly once; tool section:\n%s", toolSection)
	}
	nRunOptsLines := 0
	for _, line := range toolLines {
		if strings.Contains(line, "[runopts]") {
			nRunOptsLines++
		}
	}
	if nRunOptsLines != len(runOptsTools) {
		t.Fatalf("expected [runopts] appear in %d tool lines; got %d; tool section:\n%s", len(runOptsTools), nRunOptsLines, toolSection)
	}
}

func TestBuildSystemPrompt_FinalAnswerMustBeChinesePlainText(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "只输出中文纯文本（不要输出 <tool_data>、不要输出 XML 标签、不要输出 Markdown）"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing final-answer constraint: %q", want)
	}
}

func TestBuildSystemPrompt_WriteToolsMissingParamsShouldInfer(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "写操作若缺参，优先调用工具推断并选择安全默认"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing missing-params constraint: %q", want)
	}
}

func TestBuildSystemPrompt_TargetIncludesTaskCreation(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "任务相关操作（新建、恢复、审批、诊断、任务契约）"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing task-creation goal: %q", want)
	}
}

func TestBuildSystemPrompt_ToolLinesIncludeReturnHints(t *testing.T) {
	prompt := buildSystemPrompt()
	toolSection := extractToolSection(t, prompt)
	toolLines := parseToolLines(t, toolSection)

	type testCase struct {
		name     string
		contains []string
	}
	cases := []testCase{
		{name: "tasks_list", contains: []string{"返回：", "tasks"}},
		{name: "task_logs_tail", contains: []string{"返回：", "logs"}},
		{name: "task_log_get", contains: []string{"返回：", "message"}},
		{name: "task_new_submit", contains: []string{"返回：", "task"}},
		{name: "scheduler_create", contains: []string{"返回：", "schedule_id"}},
	}
	for _, tc := range cases {
		line, ok := toolLines[tc.name]
		if !ok {
			t.Fatalf("prompt missing tool line: %s", tc.name)
		}
		for _, want := range tc.contains {
			if !strings.Contains(line, want) {
				t.Fatalf("tool %q line missing %q: %q", tc.name, want, line)
			}
		}
	}
}

func TestBuildSystemPrompt_PrincipleDrivenGuidance(t *testing.T) {
	prompt := buildSystemPrompt()

	wants := []string{
		"一次只问一个关键问题",
		"多选引导 + 一句猜测",
		"worker_type 语义：claude-code=Claude Code 代理执行；codex=Codex 代理执行；exec=在本机 workdir 直接执行你提供的 shell（bash）命令",
		"不会做自然语言转译",
		"prompt 必须是可直接执行的命令字符串",
		"简单且追求速度 -> claude-code",
		"严肃/生产级迭代 -> codex",
		"不确定则先问再提",
		"执行摘要（目标/worker/验收/关键假设）",
		"exec 仅可在用户明确要求执行具体 shell 命令时使用",
	}

	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing principle guidance: %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildSystemPrompt_ToolSectionParseIsStable(t *testing.T) {
	prompt := buildSystemPrompt()
	toolSection := extractToolSection(t, prompt)
	toolLines := parseToolLines(t, toolSection)
	if len(toolLines) == 0 {
		t.Fatalf("expected non-empty tool lines")
	}
	for name, line := range toolLines {
		if strings.Contains(name, "\n") || strings.Contains(line, "\n\n") {
			t.Fatalf("unexpected newline in parsed tool line: %q -> %q", name, line)
		}
		if !strings.HasPrefix(line, "- ") {
			t.Fatalf("unexpected tool line prefix: %q", line)
		}
		if !strings.Contains(line, "：") {
			t.Fatalf("unexpected tool line missing colon: %q", line)
		}
	}
	if _, ok := toolLines["task_continue_submit"]; !ok {
		t.Fatalf("prompt missing expected tool: task_continue_submit")
	}
}

func TestBuildSystemPrompt_ToolProtocolExample_UsesPlaceholdersAndNoFakeStatusMeta(t *testing.T) {
	prompt := buildSystemPrompt()
	if strings.Contains(prompt, "<status>succeeded</status>") {
		t.Fatalf("tool protocol example should not include a real <status>succeeded</status> callsite")
	}
	if !strings.Contains(prompt, "<tool_name>TOOL_NAME</tool_name>") {
		t.Fatalf("tool protocol example should use TOOL_NAME placeholder")
	}
	if !strings.Contains(prompt, "<arg1>...</arg1>") {
		t.Fatalf("tool protocol example should show placeholder args")
	}
}

func TestBuildSystemPrompt_ToolProtocol_IncludesFormatConventionsAndForbiddenJSON(t *testing.T) {
	prompt := buildSystemPrompt()

	wants := []string{
		"参数格式约定：",
		"严禁使用 JSON 工具调用",
		"工具结果会以 <tool_result> XML 发给你：禁止原样复述该 XML",
		"不要猜测工具名",
	}
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing tool protocol guidance: %q\nprompt:\n%s", want, prompt)
		}
	}
}
