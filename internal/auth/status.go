package auth

import (
	"os"
	"path/filepath"
	"strings"
)

type FieldStatus struct {
	Effective string `json:"effective"` // "env" | "stored" | "codex" | "default" | "none"
	Masked    string `json:"masked,omitempty"`
}

type ClaudeStatus struct {
	BaseURL        FieldStatus `json:"base_url"`
	APIKey         FieldStatus `json:"api_key"`
	AuthToken      FieldStatus `json:"auth_token"`
	Model          FieldStatus `json:"model"`
	SmallFastModel FieldStatus `json:"small_fast_model"`
	Available      bool        `json:"available"`
}

type CodexStatus struct {
	APIKey          FieldStatus `json:"api_key"`
	Model           FieldStatus `json:"model"`
	ReasoningEffort FieldStatus `json:"reasoning_effort"`
	Available       bool        `json:"available"`
}

type Status struct {
	Claude ClaudeStatus `json:"claude"`
	Codex  CodexStatus  `json:"codex"`
}

func ComputeStatus(secrets Secrets) Status {
	baseURL := computeFieldStatusDisplay("ANTHROPIC_BASE_URL", secrets.AnthropicBaseURL)
	apiKey := computeFieldStatus("ANTHROPIC_API_KEY", secrets.AnthropicAPIKey)
	authToken := computeFieldStatus("ANTHROPIC_AUTH_TOKEN", secrets.AnthropicAuthToken)
	model := computeFieldStatusDisplay("ANTHROPIC_MODEL", secrets.AnthropicModel)
	smallFast := computeFieldStatusDisplay("ANTHROPIC_SMALL_FAST_MODEL", secrets.AnthropicSmallFastModel)
	openaiKey := computeCodexAuthStatus(secrets.OpenAIAPIKey)
	codexModel := computeCodexSettingStatus(secrets.CodexModel, "gpt-5.2")
	codexEffort := computeCodexSettingStatus(secrets.CodexReasoningEffort, "xhigh")

	return Status{
		Claude: ClaudeStatus{
			BaseURL:        baseURL,
			APIKey:         apiKey,
			AuthToken:      authToken,
			Model:          model,
			SmallFastModel: smallFast,
			Available:      apiKey.Effective != "none" || authToken.Effective != "none",
		},
		Codex: CodexStatus{
			APIKey:          openaiKey,
			Model:           codexModel,
			ReasoningEffort: codexEffort,
			Available:       openaiKey.Effective != "none",
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

func computeFieldStatusDisplay(envName string, stored string) FieldStatus {
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: TruncateDisplay(v, 96)}
	}
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: TruncateDisplay(stored, 96)}
	}
	return FieldStatus{Effective: "none"}
}

func TruncateDisplay(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	// Keep the beginning; it's usually what users care about (URL prefix, model name, etc).
	return s[:max-1] + "…"
}

func computeCodexSettingStatus(stored string, defaultValue string) FieldStatus {
	stored = strings.TrimSpace(stored)
	if stored != "" {
		return FieldStatus{Effective: "stored", Masked: TruncateDisplay(stored, 96)}
	}
	defaultValue = strings.TrimSpace(defaultValue)
	if defaultValue == "" {
		return FieldStatus{Effective: "none"}
	}
	return FieldStatus{Effective: "default", Masked: TruncateDisplay(defaultValue, 96)}
}

func computeCodexAuthStatus(stored string) FieldStatus {
	if v, ok := os.LookupEnv("OPENAI_API_KEY"); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: MaskSecret(v)}
	}
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: MaskSecret(stored)}
	}
	if path, ok := codexAuthFilePath(); ok {
		return FieldStatus{Effective: "codex", Masked: tildePath(path)}
	}
	return FieldStatus{Effective: "none"}
}

func codexAuthFilePath() (string, bool) {
	dir := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return "", false
		}
		dir = filepath.Join(home, ".codex")
	}
	p := filepath.Join(filepath.Clean(dir), "auth.json")
	info, err := os.Stat(p)
	if err != nil || info == nil || info.IsDir() {
		return "", false
	}
	if info.Size() <= 0 {
		return "", false
	}
	return p, true
}

func tildePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return p
	}
	home = filepath.Clean(home)
	pp := filepath.Clean(p)
	rel, err := filepath.Rel(home, pp)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return filepath.Join("~", rel)
	}
	if pp == home {
		return "~"
	}
	return p
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
