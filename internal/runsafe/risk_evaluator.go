package runsafe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/tasks"
)

const (
	riskEvaluatorPrinciplesSource    = "risk-evaluator-principles-v1"
	riskEvaluatorLLMPrinciplesSource = "risk-evaluator-llm-principles-v1"
)

type RiskInput struct {
	ActionType       string
	Prompt           string
	WorkerType       tasks.WorkerType
	UnsafeAutomation bool
	NetworkTier      tasks.NetworkTier
	HasRollbackProof bool
}

type RiskVerdict struct {
	RiskLevel     tasks.RiskLevel `json:"risk_level"`
	Decision      string          `json:"decision"`
	Rationale     string          `json:"rationale"`
	Reversible    bool            `json:"reversible"`
	Reversibility string          `json:"reversibility"`
	Signals       []string        `json:"signals,omitempty"`
	Source        string          `json:"source"`
}

type EvaluateRiskOptions struct {
	LLM         LLMBackend
	LLMTimeout  time.Duration
	MinLLMScore float64
}

type llmRiskDecision struct {
	RiskLevel     string   `json:"risk_level"`
	Decision      string   `json:"decision,omitempty"`
	Rationale     string   `json:"rationale,omitempty"`
	Signals       []string `json:"signals,omitempty"`
	Reversible    *bool    `json:"reversible,omitempty"`
	Reversibility string   `json:"reversibility,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
}

func EvaluateRisk(ctx context.Context, in RiskInput, opts EvaluateRiskOptions) RiskVerdict {
	det := evaluateRiskDeterministic(in)

	llm := opts.LLM
	if llm == nil {
		return det
	}

	timeout := opts.LLMTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	min := opts.MinLLMScore
	if min <= 0 {
		min = 0.65
	}
	llmCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := llm.Complete(llmCtx, buildLLMRiskPrompt(in, det))
	if err != nil {
		return det
	}
	parsed, ok := parseLLMRiskDecision(raw)
	if !ok {
		return det
	}
	if parsed.Confidence > 0 && parsed.Confidence < min {
		return det
	}

	return mergeRiskVerdict(det, parsed)
}

func evaluateRiskDeterministic(in RiskInput) RiskVerdict {
	actionType := strings.ToLower(strings.TrimSpace(in.ActionType))
	risk := tasks.RiskLow
	signals := make([]string, 0, 6)
	reasons := make([]string, 0, 4)

	if strings.Contains(actionType, "unsafe") {
		risk = maxRiskLevel(risk, tasks.RiskHigh)
		signals = append(signals, "action_unsafe")
		reasons = append(reasons, "unsafe action requested")
	}
	if in.UnsafeAutomation {
		risk = maxRiskLevel(risk, tasks.RiskHigh)
		signals = append(signals, "unsafe_automation")
		reasons = append(reasons, "unsafe automation enabled")
	}
	if tasks.NormalizeNetworkTier(string(in.NetworkTier)) == tasks.NetworkTierExecNet {
		risk = maxRiskLevel(risk, tasks.RiskHigh)
		signals = append(signals, "exec_net")
		reasons = append(reasons, "full network execution required")
	}

	cls := ClassifyPromptDeterministic(in.Prompt)
	switch cls.Intent {
	case IntentInstall:
		risk = maxRiskLevel(risk, tasks.RiskHigh)
		signals = append(signals, "install_prompt")
		reasons = append(reasons, "prompt implies dependency or environment install")
	case IntentSearchBrowse:
		risk = maxRiskLevel(risk, tasks.RiskMedium)
		signals = append(signals, "search_prompt")
		reasons = append(reasons, "prompt requires external browse/search context")
	case IntentCode:
		risk = maxRiskLevel(risk, tasks.RiskMedium)
		signals = append(signals, "code_change_prompt")
		reasons = append(reasons, "prompt implies code changes")
	}

	rev := true
	revLabel := "likely_reversible"
	switch risk {
	case tasks.RiskHigh:
		if in.HasRollbackProof {
			rev = true
			revLabel = "proof_available"
		} else {
			rev = false
			revLabel = "proof_missing"
		}
	case tasks.RiskMedium:
		rev = true
		revLabel = "likely_reversible"
	default:
		rev = true
		revLabel = "reversible"
	}

	decision := "allow"
	switch risk {
	case tasks.RiskHigh:
		if rev {
			decision = "review"
		} else {
			decision = "deny"
		}
	case tasks.RiskMedium:
		decision = "review"
	}

	reason := "low-risk action"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}
	return RiskVerdict{
		RiskLevel:     risk,
		Decision:      decision,
		Rationale:     reason,
		Reversible:    rev,
		Reversibility: revLabel,
		Signals:       dedupeStrings(signals),
		Source:        riskEvaluatorPrinciplesSource,
	}
}

func mergeRiskVerdict(det RiskVerdict, llm llmRiskDecision) RiskVerdict {
	out := det
	llmRisk := normalizeRiskLevel(llm.RiskLevel)
	if llmRisk != "" {
		out.RiskLevel = maxRiskLevel(det.RiskLevel, llmRisk)
	}

	if llm.Reversible != nil {
		// Fail-closed: LLM cannot make an irreversible action look reversible.
		if out.Reversible && !*llm.Reversible {
			out.Reversible = false
		}
	}
	if !out.Reversible {
		out.Reversibility = "proof_missing"
	} else if strings.TrimSpace(llm.Reversibility) != "" {
		out.Reversibility = strings.TrimSpace(llm.Reversibility)
	}

	if strings.TrimSpace(llm.Decision) != "" {
		out.Decision = maxDecision(out.Decision, strings.TrimSpace(strings.ToLower(llm.Decision)))
	}
	if strings.TrimSpace(llm.Rationale) != "" {
		out.Rationale = strings.TrimSpace(llm.Rationale)
	}
	if len(llm.Signals) > 0 {
		out.Signals = dedupeStrings(append(out.Signals, llm.Signals...))
	}

	// Keep decision aligned with risk even if LLM omitted it.
	switch out.RiskLevel {
	case tasks.RiskHigh:
		if !out.Reversible {
			out.Decision = maxDecision(out.Decision, "deny")
		} else {
			out.Decision = maxDecision(out.Decision, "review")
		}
	case tasks.RiskMedium:
		out.Decision = maxDecision(out.Decision, "review")
	default:
		if strings.TrimSpace(out.Decision) == "" {
			out.Decision = "allow"
		}
	}

	out.Source = riskEvaluatorLLMPrinciplesSource
	return out
}

func buildLLMRiskPrompt(in RiskInput, det RiskVerdict) string {
	inputJSON, _ := json.Marshal(map[string]any{
		"action_type":        strings.TrimSpace(in.ActionType),
		"prompt":             strings.TrimSpace(in.Prompt),
		"worker_type":        strings.TrimSpace(string(in.WorkerType)),
		"unsafe_automation":  in.UnsafeAutomation,
		"network_tier":       strings.TrimSpace(string(in.NetworkTier)),
		"has_rollback_proof": in.HasRollbackProof,
		"deterministic":      det,
	})
	return strings.TrimSpace(`
You are a risk evaluator for a local coding assistant.
Apply explicit principles:
1) unsafe / install / exec-net actions are high-risk.
2) lack of rollback proof makes high-risk actions less reversible.
3) on uncertainty, be conservative.

Return strict JSON only:
{"risk_level":"low|medium|high","decision":"allow|review|deny","reversible":true|false,"reversibility":"...","confidence":0.0-1.0,"signals":["..."],"rationale":"..."}

Input:
` + "\n" + strings.TrimSpace(string(inputJSON)))
}

func parseLLMRiskDecision(raw string) (llmRiskDecision, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if s == "" {
		return llmRiskDecision{}, false
	}
	var out llmRiskDecision
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return llmRiskDecision{}, false
	}
	if normalizeRiskLevel(out.RiskLevel) == "" {
		return llmRiskDecision{}, false
	}
	return out, true
}

func normalizeRiskLevel(raw string) tasks.RiskLevel {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case string(tasks.RiskLow):
		return tasks.RiskLow
	case string(tasks.RiskMedium):
		return tasks.RiskMedium
	case string(tasks.RiskHigh):
		return tasks.RiskHigh
	default:
		return ""
	}
}

func maxRiskLevel(a, b tasks.RiskLevel) tasks.RiskLevel {
	if riskRank(b) > riskRank(a) {
		return b
	}
	return a
}

func riskRank(v tasks.RiskLevel) int {
	switch v {
	case tasks.RiskHigh:
		return 3
	case tasks.RiskMedium:
		return 2
	case tasks.RiskLow:
		return 1
	default:
		return 0
	}
}

func maxDecision(a, b string) string {
	if decisionRank(b) > decisionRank(a) {
		return b
	}
	if strings.TrimSpace(a) == "" {
		return b
	}
	return a
}

func decisionRank(v string) int {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "deny":
		return 3
	case "review":
		return 2
	case "allow":
		return 1
	default:
		return 0
	}
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func MarshalRiskScope(v RiskVerdict, extra map[string]any) json.RawMessage {
	payload := map[string]any{
		"risk_level":    string(v.RiskLevel),
		"decision":      strings.TrimSpace(v.Decision),
		"reversible":    v.Reversible,
		"reversibility": strings.TrimSpace(v.Reversibility),
		"signals":       v.Signals,
		"evaluator":     strings.TrimSpace(v.Source),
		"rationale":     strings.TrimSpace(v.Rationale),
	}
	for k, val := range extra {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		payload[k] = val
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(b)
}

func FormatRiskSummary(v RiskVerdict) string {
	return fmt.Sprintf("risk=%s decision=%s reversible=%t source=%s", strings.TrimSpace(string(v.RiskLevel)), strings.TrimSpace(v.Decision), v.Reversible, strings.TrimSpace(v.Source))
}
