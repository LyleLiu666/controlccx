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

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseInt(strings.TrimSpace(r.URL.Query().Get("limit")), 0)
	offset := parseInt(strings.TrimSpace(r.URL.Query().Get("offset")), 0)
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	if limit > 500 {
		limit = 500
	}

	items := out.Skills
	if q != "" {
		needle := strings.ToLower(q)
		filtered := make([]skills.Skill, 0, len(items))
		for _, s := range items {
			if strings.Contains(strings.ToLower(s.Name), needle) {
				filtered = append(filtered, s)
			}
		}
		items = filtered
	}

	total := len(items)
	if limit > 0 {
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		items = items[offset:end]
	}

	type skillsPage struct {
		SourceRoots []string            `json:"source_roots"`
		Targets     []skills.TargetRoot `json:"targets"`
		Skills      []skills.Skill      `json:"skills"`
		Total       int                 `json:"total"`
		Offset      int                 `json:"offset"`
		Limit       int                 `json:"limit"`
	}

	writeJSON(w, skillsPage{
		SourceRoots: out.SourceRoots,
		Targets:     out.Targets,
		Skills:      items,
		Total:       total,
		Offset:      offset,
		Limit:       limit,
	})
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
