package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ClaudeLiveImport struct {
	HomeDir string `json:"home_dir"`

	SettingsPath string `json:"settings_path,omitempty"`

	Target ClaudeTarget `json:"target"`
}

func ImportClaudeLive(homeDir string) (ClaudeLiveImport, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return ClaudeLiveImport{}, errors.New("providers: claude import: home dir is required")
	}
	homeDir = filepath.Clean(homeDir)

	out := ClaudeLiveImport{HomeDir: homeDir}
	settingsPath, ok := firstExistingFile(homeDir, []string{"settings.json", "claude.json"})
	if !ok {
		return out, nil
	}

	b, err := os.ReadFile(settingsPath)
	if err != nil {
		return ClaudeLiveImport{}, fmt.Errorf("providers: claude import: read settings: %w", err)
	}
	env, err := parseClaudeEnvFromSettingsJSON(b)
	if err != nil {
		return ClaudeLiveImport{}, err
	}
	out.SettingsPath = settingsPath
	out.Target = ClaudeTarget{
		BaseURL:        strings.TrimSpace(env["ANTHROPIC_BASE_URL"]),
		APIKey:         strings.TrimSpace(env["ANTHROPIC_API_KEY"]),
		AuthToken:      strings.TrimSpace(env["ANTHROPIC_AUTH_TOKEN"]),
		Model:          strings.TrimSpace(env["ANTHROPIC_MODEL"]),
		SmallFastModel: strings.TrimSpace(env["ANTHROPIC_SMALL_FAST_MODEL"]),
	}
	return out, nil
}

func parseClaudeEnvFromSettingsJSON(b []byte) (map[string]string, error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("providers: claude import: parse json: %w", err)
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

func firstExistingFile(dir string, candidates []string) (string, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", false
	}
	dir = filepath.Clean(dir)
	for _, c := range candidates {
		name := strings.TrimSpace(c)
		if name == "" {
			continue
		}
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
