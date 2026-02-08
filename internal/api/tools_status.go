package api

import (
	"net/http"
)

func (a *API) handleToolsStatus(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tools == nil {
		http.Error(w, "tools service not configured", http.StatusNotImplemented)
		return
	}
	writeJSON(w, map[string]any{"tools": a.Tools.Status()})
}
