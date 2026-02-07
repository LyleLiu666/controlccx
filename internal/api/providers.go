package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/providers"
)

func (a *API) handleProviders(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{
		"profiles":     a.Providers.MaskedProfiles(),
		"active":       a.Providers.Active(),
		"storage_path": a.Providers.Path(),
	})
}

func (a *API) handleProvidersUpsert(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Profile providers.Profile `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p := body.Profile
	if strings.TrimSpace(p.ID) != "" {
		if existing, ok := a.Providers.Get(p.ID); ok {
			p = mergeProviderProfileForUpsert(existing, p)
		}
	}
	p, err := a.Providers.Upsert(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"profile": providers.MaskProfile(p)})
}

func mergeProviderProfileForUpsert(existing providers.Profile, incoming providers.Profile) providers.Profile {
	// Only special-case secrets to prevent clients from echoing masked placeholders
	// back into storage. Everything else is treated as a full replacement.
	incoming.Name = strings.TrimSpace(incoming.Name)
	if incoming.Name == "" {
		incoming.Name = existing.Name
	}

	if isMaskedSecretPlaceholder(incoming.Targets.Claude.APIKey) || strings.TrimSpace(incoming.Targets.Claude.APIKey) == "" {
		incoming.Targets.Claude.APIKey = existing.Targets.Claude.APIKey
	}
	if isMaskedSecretPlaceholder(incoming.Targets.Claude.AuthToken) || strings.TrimSpace(incoming.Targets.Claude.AuthToken) == "" {
		incoming.Targets.Claude.AuthToken = existing.Targets.Claude.AuthToken
	}
	if isMaskedSecretPlaceholder(incoming.Targets.Codex.APIKey) || strings.TrimSpace(incoming.Targets.Codex.APIKey) == "" {
		incoming.Targets.Codex.APIKey = existing.Targets.Codex.APIKey
	}
	if isMaskedSecretPlaceholder(incoming.Targets.Secretary.SimpleHTTP.APIKey) || strings.TrimSpace(incoming.Targets.Secretary.SimpleHTTP.APIKey) == "" {
		incoming.Targets.Secretary.SimpleHTTP.APIKey = existing.Targets.Secretary.SimpleHTTP.APIKey
	}
	if isMaskedSecretPlaceholder(incoming.Targets.Secretary.SimpleHTTP.AuthToken) || strings.TrimSpace(incoming.Targets.Secretary.SimpleHTTP.AuthToken) == "" {
		incoming.Targets.Secretary.SimpleHTTP.AuthToken = existing.Targets.Secretary.SimpleHTTP.AuthToken
	}
	return incoming
}

func isMaskedSecretPlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// auth.MaskSecret uses ellipsis for normal secrets and "**" for very short strings.
	if strings.Contains(s, "…") {
		return true
	}
	return s == "**"
}

func (a *API) handleProvidersDelete(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := a.Providers.Delete(body.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleProvidersDuplicate(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p, err := a.Providers.Duplicate(body.ID, body.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"profile": providers.MaskProfile(p)})
}

func (a *API) handleProvidersReorder(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := a.Providers.Reorder(body.IDs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleProvidersActivate(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil || a.Auth == nil {
		http.Error(w, "providers/auth store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Target             string `json:"target"`
		ID                 string `json:"id"`
		ForceLiveOverwrite bool   `json:"force_live_overwrite,omitempty"`
		ClaudeHomeDir      string `json:"claude_home_dir,omitempty"`
		CodexHomeDir       string `json:"codex_home_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Target))
	id := strings.TrimSpace(body.ID)
	if target == "" || id == "" {
		http.Error(w, "target and id are required", http.StatusBadRequest)
		return
	}
	p, ok := a.Providers.Get(id)
	if !ok {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}

	// Optional live sync (safety default is OFF; only runs when enabled on the profile).
	if err := a.maybeSyncLiveOnActivate(target, p, body.ClaudeHomeDir, body.CodexHomeDir, body.ForceLiveOverwrite); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	activated, err := a.Providers.Activate(target, id, a.Auth)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"profile":     providers.MaskProfile(activated),
		"active":      a.Providers.Active(),
		"auth_status": auth.ComputeStatus(a.Auth.Get()),
	})
}

func (a *API) maybeSyncLiveOnActivate(target string, p providers.Profile, claudeHomeDir string, codexHomeDir string, force bool) error {
	target = strings.ToLower(strings.TrimSpace(target))

	backupRoot := ""
	if a.Providers != nil && strings.TrimSpace(a.Providers.Path()) != "" {
		backupRoot = filepath.Join(filepath.Dir(a.Providers.Path()), "backups", "live")
	}

	opts := providers.LiveSyncOptions{BackupDir: backupRoot, Force: force}
	switch target {
	case "claude":
		if !p.SyncLive.Claude {
			return nil
		}
		home := strings.TrimSpace(claudeHomeDir)
		if home == "" {
			u, err := defaultClaudeHomeDir()
			if err != nil {
				return err
			}
			home = u
		}
		return providers.SyncClaudeLive(home, p.Targets.Claude, opts)
	case "codex":
		if !p.SyncLive.Codex {
			return nil
		}
		home := strings.TrimSpace(codexHomeDir)
		if home == "" {
			u, err := defaultCodexHomeDir()
			if err != nil {
				return err
			}
			home = u
		}
		return providers.SyncCodexLive(home, p.Targets.Codex, opts)
	default:
		// secretary live sync not supported (no canonical external config).
		return nil
	}
}

func (a *API) handleProvidersSpeedTest(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Target    string `json:"target"`
		ID        string `json:"id"`
		TimeoutMS int    `json:"timeout_ms,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Target))
	id := strings.TrimSpace(body.ID)
	if target == "" || id == "" {
		http.Error(w, "target and id are required", http.StatusBadRequest)
		return
	}
	p, ok := a.Providers.Get(id)
	if !ok {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}
	baseURL := ""
	switch target {
	case "claude":
		baseURL = strings.TrimSpace(p.Targets.Claude.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
	case "codex":
		baseURL = strings.TrimSpace(p.Targets.Codex.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
	default:
		http.Error(w, "unknown target", http.StatusBadRequest)
		return
	}

	timeout := 800 * time.Millisecond
	if body.TimeoutMS > 0 && body.TimeoutMS <= 30_000 {
		timeout = time.Duration(body.TimeoutMS) * time.Millisecond
	}
	res := providers.SpeedTest(r.Context(), baseURL, providers.SpeedTestOptions{Timeout: timeout})
	writeJSON(w, map[string]any{"result": res})
}

func (a *API) handleProvidersImportLive(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name          string `json:"name,omitempty"`
		ClaudeHomeDir string `json:"claude_home_dir,omitempty"`
		CodexHomeDir  string `json:"codex_home_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Current"
	}

	claudeHome := strings.TrimSpace(body.ClaudeHomeDir)
	if claudeHome == "" {
		u, err := defaultClaudeHomeDir()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		claudeHome = u
	}
	codexHome := strings.TrimSpace(body.CodexHomeDir)
	if codexHome == "" {
		u, err := defaultCodexHomeDir()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		codexHome = u
	}

	claudeImp, err := providers.ImportClaudeLive(claudeHome)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	codexImp, err := providers.ImportCodexLive(codexHome)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	profile, err := a.Providers.Upsert(providers.Profile{
		Name: name,
		Targets: providers.Targets{
			Claude: claudeImp.Target,
			Codex:  codexImp.Target,
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"profile": providers.MaskProfile(profile),
		"live": map[string]any{
			"claude": map[string]any{
				"home_dir":                   claudeImp.HomeDir,
				"settings_path":              claudeImp.SettingsPath,
				"anthropic_base_url":         claudeImp.Target.BaseURL,
				"anthropic_model":            claudeImp.Target.Model,
				"anthropic_small_fast_model": claudeImp.Target.SmallFastModel,
				"anthropic_api_key":          auth.MaskSecret(claudeImp.Target.APIKey),
				"anthropic_auth_token":       auth.MaskSecret(claudeImp.Target.AuthToken),
			},
			"codex": map[string]any{
				"home_dir":               codexImp.HomeDir,
				"auth_path":              codexImp.AuthPath,
				"config_path":            codexImp.ConfigPath,
				"openai_api_key":         auth.MaskSecret(codexImp.Target.APIKey),
				"codex_model":            codexImp.Target.Model,
				"codex_reasoning_effort": codexImp.Target.ReasoningEffort,
			},
		},
	})
}

func (a *API) handleProvidersImportEnv(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Target))
	if target != "claude" && target != "codex" && target != "secretary" {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	env := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	imported := []string{}
	appendImported := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			imported = append(imported, name)
		}
	}

	profile := providers.Profile{
		Name: map[string]string{
			"claude":    "Claude Env",
			"codex":     "Codex Env",
			"secretary": "Secretary Env",
		}[target],
	}
	switch target {
	case "claude":
		baseURL := env("ANTHROPIC_BASE_URL")
		apiKey := env("ANTHROPIC_API_KEY")
		authToken := env("ANTHROPIC_AUTH_TOKEN")
		model := env("ANTHROPIC_MODEL")
		smallFast := env("ANTHROPIC_SMALL_FAST_MODEL")
		profile.Targets.Claude = providers.ClaudeTarget{
			BaseURL:        baseURL,
			APIKey:         apiKey,
			AuthToken:      authToken,
			Model:          model,
			SmallFastModel: smallFast,
		}
		appendImported("ANTHROPIC_BASE_URL", baseURL)
		appendImported("ANTHROPIC_API_KEY", apiKey)
		appendImported("ANTHROPIC_AUTH_TOKEN", authToken)
		appendImported("ANTHROPIC_MODEL", model)
		appendImported("ANTHROPIC_SMALL_FAST_MODEL", smallFast)
	case "codex":
		baseURL := env("OPENAI_BASE_URL")
		apiKey := env("OPENAI_API_KEY")
		model := env("CODEX_MODEL")
		effort := env("CODEX_REASONING_EFFORT")
		profile.Targets.Codex = providers.CodexTarget{
			BaseURL:         baseURL,
			APIKey:          apiKey,
			Model:           model,
			ReasoningEffort: effort,
		}
		appendImported("OPENAI_BASE_URL", baseURL)
		appendImported("OPENAI_API_KEY", apiKey)
		appendImported("CODEX_MODEL", model)
		appendImported("CODEX_REASONING_EFFORT", effort)
	case "secretary":
		baseURL := env("ANTHROPIC_BASE_URL")
		apiKey := env("ANTHROPIC_API_KEY")
		authToken := env("ANTHROPIC_AUTH_TOKEN")
		model := env("ANTHROPIC_MODEL")
		profile.Targets.Secretary = providers.SecretaryTarget{
			Backend: "simple-http",
			SimpleHTTP: providers.SecretarySimpleHTTP{
				BaseURL:   baseURL,
				APIKey:    apiKey,
				AuthToken: authToken,
				Model:     model,
			},
		}
		appendImported("ANTHROPIC_BASE_URL", baseURL)
		appendImported("ANTHROPIC_API_KEY", apiKey)
		appendImported("ANTHROPIC_AUTH_TOKEN", authToken)
		appendImported("ANTHROPIC_MODEL", model)
	}
	writeJSON(w, map[string]any{
		"profile":  profile,
		"imported": imported,
	})
}

func (a *API) handleProvidersExport(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Providers == nil {
		http.Error(w, "providers store not configured", http.StatusServiceUnavailable)
		return
	}

	includeSecrets := strings.TrimSpace(r.URL.Query().Get("include_secrets")) == "1"
	if !includeSecrets {
		writeJSON(w, map[string]any{
			"profiles": a.Providers.MaskedProfiles(),
			"active":   a.Providers.Active(),
			"hint":     "set include_secrets=1 to export raw secrets",
		})
		return
	}
	writeJSON(w, map[string]any{"profiles": a.Providers.Profiles(), "active": a.Providers.Active()})
}

func defaultClaudeHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("providers: claude home dir unavailable")
	}
	return filepath.Join(filepath.Clean(home), ".claude"), nil
}

func defaultCodexHomeDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return filepath.Clean(v), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", errors.New("providers: codex home dir unavailable")
	}
	return filepath.Join(filepath.Clean(home), ".codex"), nil
}
