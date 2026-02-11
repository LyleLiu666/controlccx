package secretary

import (
	"strings"
	"testing"

	sectools "controlccx/internal/secretary/tools"
)

func TestBuildSystemPrompt_ContainsAllToolDescriptors(t *testing.T) {
	prompt := buildSystemPrompt()
	for _, d := range sectools.Descriptors() {
		name := strings.TrimSpace(d.Name)
		desc := strings.TrimSpace(d.DescriptionZH)
		if name == "" {
			t.Fatalf("found empty tool name")
		}
		if desc == "" {
			t.Fatalf("tool %q has empty Chinese description", name)
		}
		if !strings.Contains(prompt, name+"："+desc) {
			t.Fatalf("prompt missing tool descriptor: %s", name)
		}
	}
}

func TestBuildSystemPrompt_FinalAnswerMustBeChinesePlainText(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "只输出中文纯文本（不要输出 <tool_data>、不要输出 XML 标签、不要输出 Markdown）"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing final-answer constraint: %q", want)
	}
}

func TestBuildSystemPrompt_WriteToolsMissingParamsMustAskUser(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "对写操作工具（如创建任务）若必填参数缺失，必须先向用户索取，禁止猜测/自动补全。"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing missing-params constraint: %q", want)
	}
}

func TestBuildSystemPrompt_TargetIncludesTaskCreation(t *testing.T) {
	prompt := buildSystemPrompt()
	want := "任务相关操作（新建、恢复、审批、诊断）"
	if !strings.Contains(prompt, want) {
		t.Fatalf("prompt missing task-creation goal: %q", want)
	}
}

func TestBuildSystemPrompt_PrincipleDrivenGuidance(t *testing.T) {
	prompt := buildSystemPrompt()

	wants := []string{
		"一次只问一个关键问题",
		"多选引导 + 一句猜测",
		"简单且追求速度 -> claude-code",
		"严肃/生产级迭代 -> codex",
		"不确定则先问再提",
		"执行摘要（目标/worker/验收）",
		"exec 仅可在用户明确要求 shell/脚本执行时使用",
	}

	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing principle guidance: %q", want)
		}
	}
}
