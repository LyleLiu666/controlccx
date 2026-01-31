package runsafe

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type LLMBackend interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

type llmDecision struct {
	Intent     string   `json:"intent"`
	Confidence float64  `json:"confidence,omitempty"`
	Signals    []string `json:"signals,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

type ClassifyOptions struct {
	LLM         LLMBackend
	LLMTimeout  time.Duration
	MinLLMScore float64
}

func ClassifyPrompt(ctx context.Context, prompt string, opts ClassifyOptions) Decision {
	det := ClassifyPromptDeterministic(prompt)

	llm := opts.LLM
	if llm == nil {
		return det
	}
	if det.Intent == IntentInstall {
		// Never attempt to refine install; keep the deterministic high-confidence decision.
		return det
	}

	min := opts.MinLLMScore
	if min <= 0 {
		min = 0.65
	}
	if det.Confidence >= min {
		return det
	}

	timeout := opts.LLMTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := llm.Complete(ctx, buildLLMClassifyPrompt(prompt))
	if err != nil {
		return det
	}
	parsed, ok := parseLLMDecision(raw)
	if !ok {
		return det
	}

	intent := normalizeIntent(parsed.Intent)
	if intent == "" {
		return det
	}
	// Fail-closed: LLM MUST NOT escalate to install unless deterministic already did.
	if intent == IntentInstall && det.Intent != IntentInstall {
		return det
	}

	out := det
	out.Intent = intent
	if parsed.Confidence > 0 {
		out.Confidence = parsed.Confidence
	}
	if len(parsed.Signals) > 0 {
		out.Signals = parsed.Signals
	}
	if strings.TrimSpace(parsed.Reason) != "" {
		out.Reason = strings.TrimSpace(parsed.Reason)
	}
	return out
}

func buildLLMClassifyPrompt(userPrompt string) string {
	userPrompt = strings.TrimSpace(userPrompt)
	return strings.TrimSpace(`
You are a classifier for a local agent runner. Classify the user prompt into ONE intent:
- analyze
- code
- search-browse
- install

Rules:
- Choose "install" ONLY if the user is explicitly asking to install dependencies, set up environments, or run package managers.
- If unsure, choose "code".
- Output MUST be strict JSON and nothing else.

Return JSON schema:
{"intent":"analyze|code|search-browse|install","confidence":0.0-1.0,"signals":["..."],"reason":"..."}

User prompt:
` + "\n" + userPrompt + "\n")
}

func parseLLMDecision(raw string) (llmDecision, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if s == "" {
		return llmDecision{}, false
	}
	var out llmDecision
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return llmDecision{}, false
	}
	return out, true
}

func normalizeIntent(raw string) Intent {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case string(IntentAnalyze):
		return IntentAnalyze
	case string(IntentCode):
		return IntentCode
	case string(IntentSearchBrowse):
		return IntentSearchBrowse
	case string(IntentInstall):
		return IntentInstall
	default:
		return ""
	}
}

