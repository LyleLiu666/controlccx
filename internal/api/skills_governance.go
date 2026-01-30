package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"controlccx/internal/skills"
)

func normalizeSkillsTarget(raw string) skills.Target {
	raw = strings.TrimSpace(raw)
	if raw == "claude" {
		return skills.TargetClaudeCode
	}
	return skills.Target(raw)
}

func (a *API) handleSkillsSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		Name      string `json:"name"`
		Target    string `json:"target"`
		Overwrite bool   `json:"overwrite,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	target := normalizeSkillsTarget(body.Target)
	if name == "" || target == "" {
		http.Error(w, "name and target are required", http.StatusBadRequest)
		return
	}
	if err := a.Skills.Sync(r.Context(), name, target, body.Overwrite); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleSkillsTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	out, err := a.Skills.ListTools(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"tools": out})
}

func (a *API) handleSkillsOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	out, err := a.Skills.OnboardingPlan(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillsImportExisting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body skills.ImportExistingInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.SourcePath = strings.TrimSpace(body.SourcePath)
	body.Name = strings.TrimSpace(body.Name)
	body.Tool = strings.TrimSpace(body.Tool)

	if body.SourcePath == "" || body.Name == "" {
		http.Error(w, "source_path and name are required", http.StatusBadRequest)
		return
	}
	out, err := a.Skills.ImportExisting(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillsInstallLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body skills.InstallLocalInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.SourcePath = strings.TrimSpace(body.SourcePath)
	body.Name = strings.TrimSpace(body.Name)
	out, err := a.Skills.InstallLocal(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillsGitCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		RepoURL string `json:"repo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	repoURL := strings.TrimSpace(body.RepoURL)
	if repoURL == "" {
		http.Error(w, "repo_url is required", http.StatusBadRequest)
		return
	}
	out, err := a.Skills.ListGitSkills(r.Context(), repoURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"candidates": out})
}

func (a *API) handleSkillsInstallGit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body skills.InstallGitInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.RepoURL = strings.TrimSpace(body.RepoURL)
	body.Subpath = strings.TrimSpace(body.Subpath)
	body.Name = strings.TrimSpace(body.Name)

	out, err := a.Skills.InstallGit(r.Context(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, out)
}

func (a *API) handleSkillsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Skills == nil {
		http.Error(w, "skills service not configured", http.StatusNotImplemented)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	out, err := a.Skills.UpdateManagedSkill(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, out)
}
