package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"controlccx/internal/skills"
)

func (a *API) handleSkillScopedRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimSpace(parts[0])
	switch parts[1] {
	case "versions":
		a.handleSkillVersionsBySkill(w, r, name, parts[2:])
	default:
		http.NotFound(w, r)
	}
}

func (a *API) handleSkillVersionsBySkill(w http.ResponseWriter, r *http.Request, name string, rest []string) {
	if a.SkillVersionsBySkill == nil {
		http.Error(w, "skills per-skill versions service not configured", http.StatusNotImplemented)
		return
	}

	// /api/skills/{name}/versions
	if len(rest) == 0 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		out, err := a.SkillVersionsBySkill.List(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, out)
		return
	}

	// /api/skills/{name}/versions/{action}
	action := rest[0]
	switch action {
	case "create":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body skills.CreateVersionInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		body.ID = strings.TrimSpace(body.ID)
		body.Note = strings.TrimSpace(body.Note)
		v, err := a.SkillVersionsBySkill.Create(r.Context(), name, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, v)
	case "delete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		if err := a.SkillVersionsBySkill.Delete(r.Context(), name, id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case "restore":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body skills.RestoreVersionInput
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		body.ID = strings.TrimSpace(body.ID)
		body.BackupNote = strings.TrimSpace(body.BackupNote)
		out, err := a.SkillVersionsBySkill.Restore(r.Context(), name, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if a.Skills != nil && strings.TrimSpace(out.Path) != "" {
			if err := a.Skills.ResyncManagedCopies(r.Context(), name, out.Path); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, out)
	default:
		http.NotFound(w, r)
	}
}
