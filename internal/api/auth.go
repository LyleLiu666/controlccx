package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"controlccx/internal/auth"
	"controlccx/internal/daemon"
)

func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if !a.allowSensitiveRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secrets := auth.Secrets{}
	if a.Auth != nil {
		secrets = a.Auth.Get()
	}
	writeJSON(w, auth.ComputeStatus(secrets))
}

func (a *API) handleAuth(w http.ResponseWriter, r *http.Request) {
	if !a.allowSensitiveRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch r.Method {
	case http.MethodGet:
		secrets := auth.Secrets{}
		path := ""
		if a.Auth != nil {
			secrets = a.Auth.Get()
			path = a.Auth.Path()
		}
		writeJSON(w, map[string]any{
			"status":       auth.ComputeStatus(secrets),
			"storage_path": path,
		})
	case http.MethodPost:
		if a.Auth == nil {
			http.Error(w, "auth store not configured", http.StatusServiceUnavailable)
			return
		}
		var patch auth.Patch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		secrets, err := a.Auth.ApplyPatch(patch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"status":       auth.ComputeStatus(secrets),
			"storage_path": a.Auth.Path(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleAuthImportEnv(w http.ResponseWriter, r *http.Request) {
	if !a.allowSensitiveRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Auth == nil {
		http.Error(w, "auth store not configured", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Target string `json:"target,omitempty"` // "claude" | "codex" | "all"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Target))
	if target == "" {
		target = "all"
	}
	if target != "all" && target != "claude" && target != "codex" {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	current := a.Auth.Get()
	var patch auth.Patch
	imported := []string{}
	skipped := []string{}

	importField := func(fieldName string, envName string, stored string, set func(v *string)) {
		raw, ok := os.LookupEnv(envName)
		if !ok {
			return
		}
		v := strings.TrimSpace(raw)
		if v == "" {
			return
		}
		if strings.TrimSpace(stored) != "" {
			skipped = append(skipped, fieldName)
			return
		}
		vv := v
		set(&vv)
		imported = append(imported, fieldName)
	}

	if target == "all" || target == "claude" {
		importField("anthropic_base_url", "ANTHROPIC_BASE_URL", current.AnthropicBaseURL, func(v *string) { patch.AnthropicBaseURL = v })
		importField("anthropic_api_key", "ANTHROPIC_API_KEY", current.AnthropicAPIKey, func(v *string) { patch.AnthropicAPIKey = v })
		importField("anthropic_auth_token", "ANTHROPIC_AUTH_TOKEN", current.AnthropicAuthToken, func(v *string) { patch.AnthropicAuthToken = v })
		importField("anthropic_model", "ANTHROPIC_MODEL", current.AnthropicModel, func(v *string) { patch.AnthropicModel = v })
		importField("anthropic_small_fast_model", "ANTHROPIC_SMALL_FAST_MODEL", current.AnthropicSmallFastModel, func(v *string) {
			patch.AnthropicSmallFastModel = v
		})
	}

	if target == "all" || target == "codex" {
		importField("openai_api_key", "OPENAI_API_KEY", current.OpenAIAPIKey, func(v *string) { patch.OpenAIAPIKey = v })
	}

	if len(imported) > 0 {
		if _, err := a.Auth.ApplyPatch(patch); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	secrets := a.Auth.Get()
	writeJSON(w, map[string]any{
		"status":       auth.ComputeStatus(secrets),
		"storage_path": a.Auth.Path(),
		"imported":     imported,
		"skipped":      skipped,
	})
}

func (a *API) allowSensitiveRequest(r *http.Request) bool {
	if isLoopbackRequest(r) {
		return true
	}
	// Allow remote access only when the caller presents the instance token.
	// This is required for configuring auth when controlccx binds to non-loopback interfaces.
	instanceToken := strings.TrimSpace(a.InstanceToken)
	if instanceToken == "" {
		return false
	}
	return daemon.HasValidInstanceToken(r.Header, instanceToken)
}

func isLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
