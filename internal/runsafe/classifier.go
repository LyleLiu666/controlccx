package runsafe

import (
	"strings"
)

func ClassifyPromptDeterministic(prompt string) Decision {
	raw := strings.TrimSpace(prompt)
	if raw == "" {
		return Decision{
			Intent:     IntentCode,
			Confidence: 0.2,
			Signals:    []string{"default"},
			Reason:     "empty prompt",
		}
	}
	return Decision{
		Intent:     IntentCode,
		Confidence: 0.2,
		Signals:    []string{"default"},
		Reason:     "default to code",
	}
}
