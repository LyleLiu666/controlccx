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
