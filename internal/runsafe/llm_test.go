package runsafe

import (
	"context"
	"testing"
)

type stubLLM struct {
	name string
	out  string
	err  error
}

func (s *stubLLM) Name() string { return s.name }

func (s *stubLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return s.out, s.err
}

func TestClassifyPrompt_LLMRefine(t *testing.T) {
	llm := &stubLLM{
		name: "stub",
		out:  `{"intent":"analyze","confidence":0.9,"signals":["llm"],"reason":"looks like explanation"}`,
	}
	got := ClassifyPrompt(context.Background(), "Can you explain what this does?", ClassifyOptions{LLM: llm})
	if got.Intent != IntentAnalyze {
		t.Fatalf("intent=%q, want %q", got.Intent, IntentAnalyze)
	}
}

func TestClassifyPrompt_LLMInstallRequiresHighConfidence(t *testing.T) {
	llm := &stubLLM{
		name: "stub",
		out:  `{"intent":"install","confidence":0.75,"reason":"uncertain"}`,
	}
	got := ClassifyPrompt(context.Background(), "Please help me with this code", ClassifyOptions{LLM: llm})
	if got.Intent != IntentCode {
		t.Fatalf("intent=%q, want %q", got.Intent, IntentCode)
	}
}
