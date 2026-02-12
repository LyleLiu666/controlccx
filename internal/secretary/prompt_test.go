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

func TestBuildSystemPrompt_PrincipleDrivenGuidance(t *testing.T) {
	prompt := buildSystemPrompt()

	wants := []string{
		"一次只问一个关键问题",
		"多选引导 + 一句猜测",
		"worker_type 语义：claude-code=Claude Code 代理执行；codex=Codex 代理执行；exec=在本机 workdir 直接执行 shell/脚本（由 worker 进程执行，不是秘书自身执行）。",
		"简单且追求速度 -> claude-code",
		"严肃/生产级迭代 -> codex",
		"不确定则先问再提",
		"执行摘要（目标/worker/验收/关键假设）",
		"exec 仅可在用户明确要求 shell/脚本执行时使用",
	}

	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing principle guidance: %q", want)
		}
	}
}
