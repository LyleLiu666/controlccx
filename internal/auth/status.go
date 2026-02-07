package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type FieldStatus struct {
	Effective string `json:"effective"` // "env" | "stored" | "live" | "default" | "none"
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
	Claude   ClaudeStatus `json:"claude"`
	Codex    CodexStatus  `json:"codex"`
	Warnings []string     `json:"warnings,omitempty"`
}

func ComputeStatus(secrets Secrets) Status {
	claudeLive := readClaudeLiveEnv()
	baseURL := computeFieldStatusDisplay("ANTHROPIC_BASE_URL", secrets.AnthropicBaseURL, claudeLive["ANTHROPIC_BASE_URL"])
	apiKey := computeFieldStatus("ANTHROPIC_API_KEY", secrets.AnthropicAPIKey, claudeLive["ANTHROPIC_API_KEY"])
	authToken := computeFieldStatus("ANTHROPIC_AUTH_TOKEN", secrets.AnthropicAuthToken, claudeLive["ANTHROPIC_AUTH_TOKEN"])
	model := computeFieldStatusDisplay("ANTHROPIC_MODEL", secrets.AnthropicModel, claudeLive["ANTHROPIC_MODEL"])
	smallFast := computeFieldStatusDisplay("ANTHROPIC_SMALL_FAST_MODEL", secrets.AnthropicSmallFastModel, claudeLive["ANTHROPIC_SMALL_FAST_MODEL"])
	openaiKey := computeCodexAuthStatus(secrets.OpenAIAPIKey)
	codexModel := computeCodexSettingStatus(secrets.CodexModel, "gpt-5.2")
	codexEffort := computeCodexSettingStatus(secrets.CodexReasoningEffort, "xhigh")

	var warnings []string
	for _, name := range []string{
		"ANTHROPIC_BASE_URL",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_MODEL",
		"ANTHROPIC_SMALL_FAST_MODEL",
		"OPENAI_API_KEY",
	} {
		if v, ok := os.LookupEnv(name); ok && strings.TrimSpace(v) != "" {
			warnings = append(warnings, "env import: "+name+" is set; 可导入到新配置")
		}
	}

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
		Warnings: warnings,
	}
}

func computeFieldStatus(envName string, stored string, live string) FieldStatus {
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: MaskSecret(stored)}
	}
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: MaskSecret(v)}
	}
	if strings.TrimSpace(live) != "" {
		return FieldStatus{Effective: "live", Masked: MaskSecret(live)}
	}
	return FieldStatus{Effective: "none"}
}

func computeFieldStatusDisplay(envName string, stored string, live string) FieldStatus {
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: TruncateDisplay(stored, 96)}
	}
	if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: TruncateDisplay(v, 96)}
	}
	if strings.TrimSpace(live) != "" {
		return FieldStatus{Effective: "live", Masked: TruncateDisplay(live, 96)}
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
	if strings.TrimSpace(stored) != "" {
		return FieldStatus{Effective: "stored", Masked: MaskSecret(stored)}
	}
	if v, ok := os.LookupEnv("OPENAI_API_KEY"); ok && strings.TrimSpace(v) != "" {
		return FieldStatus{Effective: "env", Masked: MaskSecret(v)}
	}
	if path, ok := codexAuthFilePath(); ok {
		return FieldStatus{Effective: "live", Masked: tildePath(path)}
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

func readClaudeLiveEnv() map[string]string {
	path, ok := claudeSettingsFilePath()
	if !ok {
		return map[string]string{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	env, err := parseClaudeEnvFromSettingsJSON(b)
	if err != nil {
		return map[string]string{}
	}
	return env
}

func claudeSettingsFilePath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	dir := filepath.Join(filepath.Clean(home), ".claude")
	for _, name := range []string{"settings.json", "claude.json"} {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil || info == nil || info.IsDir() {
			continue
		}
		if info.Size() <= 0 {
			continue
		}
		return p, true
	}
	return "", false
}

func parseClaudeEnvFromSettingsJSON(b []byte) (map[string]string, error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	raw, ok := v["env"].(map[string]any)
	if !ok {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(raw))
	for k, vv := range raw {
		ks := strings.TrimSpace(k)
		if ks == "" {
			continue
		}
		s, ok := vv.(string)
		if !ok {
			continue
		}
		out[ks] = s
	}
	return out, nil
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
