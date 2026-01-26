package api

import (
	"encoding/json"
	"net"
	"net/http"

	"controlccx/internal/auth"
)

func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
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
	if !isLoopbackRequest(r) {
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
