package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"controlccx/internal/skills"
)

func (a *API) handleSkillVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.SkillVersions == nil {
		http.Error(w, "skills versions service not configured", http.StatusNotImplemented)
		return
	}
	out, err := a.SkillVersions.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillVersionsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.SkillVersions == nil {
		http.Error(w, "skills versions service not configured", http.StatusNotImplemented)
		return
	}
	var body skills.CreateVersionInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.ID = strings.TrimSpace(body.ID)
	body.Note = strings.TrimSpace(body.Note)

	v, err := a.SkillVersions.Create(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, v)
}

func (a *API) handleSkillVersionsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.SkillVersions == nil {
		http.Error(w, "skills versions service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if err := a.SkillVersions.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
