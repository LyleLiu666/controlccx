package auth

import (
	"os"
	"strings"
)

type FieldStatus struct {
	Effective string `json:"effective"` // "env" | "stored" | "none"
	Masked    string `json:"masked,omitempty"`
}

type ClaudeStatus struct {
	APIKey    FieldStatus `json:"api_key"`
	AuthToken FieldStatus `json:"auth_token"`
	Available bool        `json:"available"`
}

type CodexStatus struct {
	APIKey    FieldStatus `json:"api_key"`
	Available bool        `json:"available"`
}

type Status struct {
	Claude ClaudeStatus `json:"claude"`
	Codex  CodexStatus  `json:"codex"`
}

func ComputeStatus(secrets Secrets) Status {
	apiKey := computeFieldStatus("ANTHROPIC_API_KEY", secrets.AnthropicAPIKey)
	authToken := computeFieldStatus("ANTHROPIC_AUTH_TOKEN", secrets.AnthropicAuthToken)
	openaiKey := computeFieldStatus("OPENAI_API_KEY", secrets.OpenAIAPIKey)

	return Status{
		Claude: ClaudeStatus{
			APIKey:    apiKey,
			AuthToken: authToken,
			Available: apiKey.Effective != "none" || authToken.Effective != "none",
		},
		Codex: CodexStatus{
			APIKey:    openaiKey,
			Available: openaiKey.Effective != "none",
		},
	}
}

func computeFieldStatus(envName string, stored string) FieldStatus {
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: MaskSecret(v)}
	}
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: MaskSecret(stored)}
	}
	return FieldStatus{Effective: "none"}
}

func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Show a stable hint without leaking the full secret.
	const (
		keepPrefix = 3
		keepSuffix = 4
	)
	if len(s) <= keepPrefix+keepSuffix {
		if len(s) <= 2 {
			return "**"
		}
		return s[:1] + "…" + s[len(s)-1:]
	}
	return s[:keepPrefix] + "…" + s[len(s)-keepSuffix:]
}

