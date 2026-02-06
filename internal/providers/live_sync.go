package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type LiveSyncOptions struct {
	BackupDir string // root dir for backups (e.g. data-dir/backups/live)
	Keep      int    // default: DefaultBackupKeep
	Force     bool   // if true, allow overwriting on parse errors (still best-effort)
}

func (o LiveSyncOptions) normalized() LiveSyncOptions {
	o.BackupDir = strings.TrimSpace(o.BackupDir)
	if o.BackupDir != "" {
		o.BackupDir = filepath.Clean(o.BackupDir)
	}
	if o.Keep <= 0 {
		o.Keep = DefaultBackupKeep
	}
	return o
}

func SyncCodexLive(codexHomeDir string, target CodexTarget, opts LiveSyncOptions) error {
	codexHomeDir = strings.TrimSpace(codexHomeDir)
	if codexHomeDir == "" {
		return errors.New("providers: codex sync: home dir is required")
	}
	codexHomeDir = filepath.Clean(codexHomeDir)
	target = normalizeCodexTarget(target)
	opts = opts.normalized()
	if target.APIKey == "" {
		return errors.New("providers: codex sync: api key is required")
	}

	authPath := filepath.Join(codexHomeDir, "auth.json")
	authBackupDir := ""
	configBackupDir := ""
	if opts.BackupDir != "" {
		authBackupDir = filepath.Join(opts.BackupDir, "codex", "auth")
		configBackupDir = filepath.Join(opts.BackupDir, "codex", "config")
	}
	if err := syncCodexAuthJSON(authPath, target.APIKey, authBackupDir, opts); err != nil {
		return err
	}
	configPath := filepath.Join(codexHomeDir, "config.toml")
	if err := syncCodexConfigTOML(configPath, target.Model, target.ReasoningEffort, configBackupDir, opts); err != nil {
		return err
	}
	return nil
}

func SyncClaudeLive(claudeHomeDir string, target ClaudeTarget, opts LiveSyncOptions) error {
	claudeHomeDir = strings.TrimSpace(claudeHomeDir)
	if claudeHomeDir == "" {
		return errors.New("providers: claude sync: home dir is required")
	}
	claudeHomeDir = filepath.Clean(claudeHomeDir)
	target = normalizeClaudeTarget(target)
	opts = opts.normalized()

	settingsPath := pickClaudeSettingsPath(claudeHomeDir)
	backupDir := ""
	if opts.BackupDir != "" {
		backupDir = filepath.Join(opts.BackupDir, "claude")
	}
	return syncClaudeSettingsJSON(settingsPath, target, backupDir, opts)
}

func syncCodexAuthJSON(authPath string, apiKey string, backupDir string, opts LiveSyncOptions) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return errors.New("providers: codex sync: api key is required")
	}
	backupDir = strings.TrimSpace(backupDir)
	opts = opts.normalized()

	var obj map[string]any
	hasExisting := false
	if b, err := os.ReadFile(authPath); err == nil {
		hasExisting = true
		if err := json.Unmarshal(b, &obj); err != nil {
			if !opts.Force {
				return fmt.Errorf("providers: codex sync: parse auth.json: %w", err)
			}
			obj = map[string]any{}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("providers: codex sync: read auth.json: %w", err)
	}
	if obj == nil {
		obj = map[string]any{}
	}

	obj["OPENAI_API_KEY"] = apiKey

	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("providers: codex sync: marshal auth.json: %w", err)
	}
	data = append(data, '\n')

	if hasExisting && backupDir != "" {
		if _, err := CreateRotatingBackup(authPath, backupDir, opts.Keep); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(authPath), 0o755); err != nil {
		return fmt.Errorf("providers: codex sync: ensure dir: %w", err)
	}
	if err := writeFileAtomic(authPath, data, 0o600); err != nil {
		return fmt.Errorf("providers: codex sync: write auth.json: %w", err)
	}
	return nil
}

func syncCodexConfigTOML(configPath string, model string, effort string, backupDir string, opts LiveSyncOptions) error {
	model = strings.TrimSpace(model)
	effort = strings.TrimSpace(effort)
	backupDir = strings.TrimSpace(backupDir)
	opts = opts.normalized()
	if model == "" && effort == "" {
		return nil
	}

	b, err := os.ReadFile(configPath)
	hasExisting := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("providers: codex sync: read config.toml: %w", err)
	}

	next := string(b)
	if model != "" {
		next = upsertTopLevelTOMLString(next, "model", model)
	}
	if effort != "" {
		next = upsertTopLevelTOMLString(next, "model_reasoning_effort", effort)
	}
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}

	if hasExisting && backupDir != "" {
		if _, err := CreateRotatingBackup(configPath, backupDir, opts.Keep); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("providers: codex sync: ensure dir: %w", err)
	}
	if err := writeFileAtomic(configPath, []byte(next), 0o600); err != nil {
		return fmt.Errorf("providers: codex sync: write config.toml: %w", err)
	}
	return nil
}

func upsertTopLevelTOMLString(src string, key string, value string) string {
	src = strings.TrimSpace(src)
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return src
	}

	lines := strings.Split(src, "\n")
	limit := len(lines)
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			limit = i
			break
		}
	}

	needle := key + " ="
	replaced := false
	for i := 0; i < limit; i++ {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.HasPrefix(trim, needle) {
			lines[i] = fmt.Sprintf("%s = %q", key, value)
			replaced = true
			break
		}
	}
	if replaced {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	// Insert after leading comments/blanks.
	insertAt := 0
	for insertAt < limit {
		trim := strings.TrimSpace(lines[insertAt])
		if trim == "" || strings.HasPrefix(trim, "#") {
			insertAt++
			continue
		}
		break
	}
	head := append([]string{}, lines[:insertAt]...)
	head = append(head, fmt.Sprintf("%s = %q", key, value))
	head = append(head, lines[insertAt:]...)
	return strings.TrimSpace(strings.Join(head, "\n"))
}

func pickClaudeSettingsPath(claudeHomeDir string) string {
	claudeHomeDir = strings.TrimSpace(claudeHomeDir)
	if claudeHomeDir == "" {
		return ""
	}
	claudeHomeDir = filepath.Clean(claudeHomeDir)
	for _, name := range []string{"settings.json", "claude.json"} {
		p := filepath.Join(claudeHomeDir, name)
		if info, err := os.Stat(p); err == nil && info != nil && !info.IsDir() {
			return p
		}
	}
	return filepath.Join(claudeHomeDir, "settings.json")
}

func syncClaudeSettingsJSON(settingsPath string, target ClaudeTarget, backupDir string, opts LiveSyncOptions) error {
	settingsPath = strings.TrimSpace(settingsPath)
	if settingsPath == "" {
		return errors.New("providers: claude sync: settings path is required")
	}
	backupDir = strings.TrimSpace(backupDir)
	opts = opts.normalized()

	var obj map[string]any
	hasExisting := false
	if b, err := os.ReadFile(settingsPath); err == nil {
		hasExisting = true
		if err := json.Unmarshal(b, &obj); err != nil {
			if !opts.Force {
				return fmt.Errorf("providers: claude sync: parse settings.json: %w", err)
			}
			obj = map[string]any{}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("providers: claude sync: read settings.json: %w", err)
	}
	if obj == nil {
		obj = map[string]any{}
	}

	env, _ := obj["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	if target.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = target.BaseURL
	}
	if target.APIKey != "" {
		env["ANTHROPIC_API_KEY"] = target.APIKey
	}
	if target.AuthToken != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = target.AuthToken
	}
	if target.Model != "" {
		env["ANTHROPIC_MODEL"] = target.Model
	}
	if target.SmallFastModel != "" {
		env["ANTHROPIC_SMALL_FAST_MODEL"] = target.SmallFastModel
	}
	obj["env"] = env

	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("providers: claude sync: marshal settings.json: %w", err)
	}
	data = append(data, '\n')

	if hasExisting && backupDir != "" {
		if _, err := CreateRotatingBackup(settingsPath, backupDir, opts.Keep); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("providers: claude sync: ensure dir: %w", err)
	}
	if err := writeFileAtomic(settingsPath, data, 0o600); err != nil {
		return fmt.Errorf("providers: claude sync: write settings.json: %w", err)
	}
	return nil
}

func normalizeClaudeTarget(t ClaudeTarget) ClaudeTarget {
	t.BaseURL = strings.TrimSpace(t.BaseURL)
	t.APIKey = strings.TrimSpace(t.APIKey)
	t.AuthToken = strings.TrimSpace(t.AuthToken)
	t.Model = strings.TrimSpace(t.Model)
	t.SmallFastModel = strings.TrimSpace(t.SmallFastModel)
	return t
}

func normalizeCodexTarget(t CodexTarget) CodexTarget {
	t.BaseURL = strings.TrimSpace(t.BaseURL)
	t.APIKey = strings.TrimSpace(t.APIKey)
	t.Model = strings.TrimSpace(t.Model)
	t.ReasoningEffort = strings.TrimSpace(t.ReasoningEffort)
	return t
}
