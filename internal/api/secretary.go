package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (a *API) handleSecretaryMessages(w http.ResponseWriter, r *http.Request) {
	if a.Secretary == nil {
		http.Error(w, "secretary not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := parseInt(r.URL.Query().Get("limit"), 200)
		msgs, err := a.Secretary.History(r.Context(), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"messages": msgs})
	case http.MethodPost:
		var body struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		msg := strings.TrimSpace(body.Message)
		if msg == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}
		reply, err := a.Secretary.Send(r.Context(), msg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"reply": reply})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleSecretaryClear(w http.ResponseWriter, r *http.Request) {
	if a.Secretary == nil {
		http.Error(w, "secretary not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.Secretary.Clear(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
