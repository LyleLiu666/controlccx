package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type CodexLiveImport struct {
	HomeDir string `json:"home_dir"`

	AuthPath   string `json:"auth_path,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`

	Target CodexTarget `json:"target"`
}

func ImportCodexLive(homeDir string) (CodexLiveImport, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return CodexLiveImport{}, errors.New("providers: codex import: home dir is required")
	}
	homeDir = filepath.Clean(homeDir)

	out := CodexLiveImport{HomeDir: homeDir}

	authPath := filepath.Join(homeDir, "auth.json")
	if b, err := os.ReadFile(authPath); err == nil {
		out.AuthPath = authPath
		apiKey, err := parseCodexAuthJSON(b)
		if err != nil {
			return CodexLiveImport{}, fmt.Errorf("providers: codex import: parse auth.json: %w", err)
		}
		out.Target.APIKey = strings.TrimSpace(apiKey)
	} else if !os.IsNotExist(err) {
		return CodexLiveImport{}, fmt.Errorf("providers: codex import: read auth.json: %w", err)
	}

	configPath := filepath.Join(homeDir, "config.toml")
	if b, err := os.ReadFile(configPath); err == nil {
		out.ConfigPath = configPath
		cfg := parseCodexConfigTOML(b)
		out.Target.Model = cfg.Model
		out.Target.ReasoningEffort = cfg.ReasoningEffort
	} else if !os.IsNotExist(err) {
		return CodexLiveImport{}, fmt.Errorf("providers: codex import: read config.toml: %w", err)
	}

	return out, nil
}

func parseCodexAuthJSON(b []byte) (openaiAPIKey string, err error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return "", err
	}
	if s, ok := v["OPENAI_API_KEY"].(string); ok && strings.TrimSpace(s) != "" {
		return s, nil
	}
	return "", nil
}

type codexConfig struct {
	Model           string
	ReasoningEffort string
}

var (
	reCodexModel  = regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"\s*$`)
	reCodexEffort = regexp.MustCompile(`(?m)^\s*model_reasoning_effort\s*=\s*"([^"]+)"\s*$`)
)

func parseCodexConfigTOML(b []byte) codexConfig {
	s := string(b)
	cfg := codexConfig{}
	if m := reCodexModel.FindStringSubmatch(s); len(m) == 2 {
		cfg.Model = strings.TrimSpace(m[1])
	}
	if m := reCodexEffort.FindStringSubmatch(s); len(m) == 2 {
		cfg.ReasoningEffort = strings.TrimSpace(m[1])
	}
	return cfg
}
