package runsafe

import (
	"context"
	"encoding/json"
	"testing"

	"controlccx/internal/tasks"
)

func TestEvaluateRisk_DeterministicHighWithoutRollbackProof(t *testing.T) {
	got := EvaluateRisk(context.Background(), RiskInput{
		ActionType:       "task.enter_unsafe",
		Prompt:           "continue and install dependencies",
		WorkerType:       tasks.WorkerClaudeCode,
		UnsafeAutomation: true,
		NetworkTier:      tasks.NetworkTierExecNet,
		HasRollbackProof: false,
	}, EvaluateRiskOptions{})

	if got.RiskLevel != tasks.RiskHigh {
		t.Fatalf("risk_level=%q, want %q", got.RiskLevel, tasks.RiskHigh)
	}
	if got.Reversible {
		t.Fatalf("reversible=%v, want false", got.Reversible)
	}
	if got.Reversibility != "proof_missing" {
		t.Fatalf("reversibility=%q, want %q", got.Reversibility, "proof_missing")
	}
	if got.Decision != "deny" {
		t.Fatalf("decision=%q, want %q", got.Decision, "deny")
	}
}

func TestEvaluateRisk_DeterministicHighWithRollbackProof(t *testing.T) {
	got := EvaluateRisk(context.Background(), RiskInput{
		ActionType:       "task.enter_unsafe",
		Prompt:           "continue",
		WorkerType:       tasks.WorkerClaudeCode,
		UnsafeAutomation: true,
		NetworkTier:      tasks.NetworkTierExecNet,
		HasRollbackProof: true,
	}, EvaluateRiskOptions{})

	if got.RiskLevel != tasks.RiskHigh {
		t.Fatalf("risk_level=%q, want %q", got.RiskLevel, tasks.RiskHigh)
	}
	if !got.Reversible {
		t.Fatalf("reversible=%v, want true", got.Reversible)
	}
	if got.Reversibility != "proof_available" {
		t.Fatalf("reversibility=%q, want %q", got.Reversibility, "proof_available")
	}
	if got.Decision != "review" {
		t.Fatalf("decision=%q, want %q", got.Decision, "review")
	}
}

func TestEvaluateRisk_LLMCannotDowngradeDeterministicRisk(t *testing.T) {
	llm := &stubLLM{
		name: "stub",
		out:  `{"risk_level":"low","decision":"allow","confidence":0.95,"reversible":true,"reversibility":"reversible"}`,
	}
	got := EvaluateRisk(context.Background(), RiskInput{
		ActionType:       "task.enter_unsafe",
		Prompt:           "continue and install dependencies",
		WorkerType:       tasks.WorkerClaudeCode,
		UnsafeAutomation: true,
		NetworkTier:      tasks.NetworkTierExecNet,
		HasRollbackProof: false,
	}, EvaluateRiskOptions{LLM: llm})

	if got.RiskLevel != tasks.RiskHigh {
		t.Fatalf("risk_level=%q, want %q", got.RiskLevel, tasks.RiskHigh)
	}
	if got.Decision != "deny" {
		t.Fatalf("decision=%q, want %q", got.Decision, "deny")
	}
}

func TestEvaluateRisk_LLMCanEscalateAndTightenReversibility(t *testing.T) {
	llm := &stubLLM{
		name: "stub",
		out:  `{"risk_level":"high","decision":"review","reversible":false,"reversibility":"proof_missing","confidence":0.95,"signals":["llm"]}`,
	}
	got := EvaluateRisk(context.Background(), RiskInput{
		ActionType:       "task.continue",
		Prompt:           "search latest release notes",
		WorkerType:       tasks.WorkerClaudeCode,
		UnsafeAutomation: false,
		NetworkTier:      tasks.NetworkTierWebReadonly,
		HasRollbackProof: true,
	}, EvaluateRiskOptions{LLM: llm})

	if got.RiskLevel != tasks.RiskHigh {
		t.Fatalf("risk_level=%q, want %q", got.RiskLevel, tasks.RiskHigh)
	}
	if got.Reversible {
		t.Fatalf("reversible=%v, want false", got.Reversible)
	}
	if got.Reversibility != "proof_missing" {
		t.Fatalf("reversibility=%q, want %q", got.Reversibility, "proof_missing")
	}
	if got.Source != riskEvaluatorLLMPrinciplesSource {
		t.Fatalf("source=%q, want %q", got.Source, riskEvaluatorLLMPrinciplesSource)
	}
}

func TestMarshalRiskScope_ContainsReversibilityFields(t *testing.T) {
	raw := MarshalRiskScope(RiskVerdict{
		RiskLevel:     tasks.RiskHigh,
		Decision:      "review",
		Rationale:     "test",
		Reversible:    true,
		Reversibility: "proof_available",
		Signals:       []string{"a"},
		Source:        riskEvaluatorLLMPrinciplesSource,
	}, map[string]any{"action_type": "task.enter_unsafe"})

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["reversible"]; !ok {
		t.Fatalf("missing reversible: %v", got)
	}
	if _, ok := got["reversibility"]; !ok {
		t.Fatalf("missing reversibility: %v", got)
	}
}
