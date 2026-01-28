package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"controlccx/internal/skills"
)

func (a *API) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	out, err := a.Skills.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillsLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	target := skills.Target(strings.TrimSpace(body.Target))
	if name == "" || target == "" {
		http.Error(w, "name and target are required", http.StatusBadRequest)
		return
	}
	if err := a.Skills.Link(r.Context(), name, target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleSkillsUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	target := skills.Target(strings.TrimSpace(body.Target))
	if name == "" || target == "" {
		http.Error(w, "name and target are required", http.StatusBadRequest)
		return
	}
	if err := a.Skills.Unlink(r.Context(), name, target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

