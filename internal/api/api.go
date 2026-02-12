package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"controlccx/internal/audit"
	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/events"
	"controlccx/internal/providers"
	"controlccx/internal/runworkspace"
	"controlccx/internal/secretary"
	"controlccx/internal/skills"
	"controlccx/internal/systeminfo"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
)

type API struct {
	Tasks                *tasks.Store
	Workers              TaskRunner
	Secretary            *secretary.Service
	Audit                *audit.Service
	Chat                 *chat.Store
	Hub                  *events.Hub
	FSRoots              []FSRoot
	Auth                 *auth.Store
	InstanceToken        string
	Providers            *providers.Store
	Skills               *skills.Service
	SkillVersions        *skills.VersionsService
	SkillVersionsBySkill *skills.PerSkillVersionsService
	SkillAutoVersionScan *skills.AutoVersionScanner
	Tools                *tooling.Service
	Workspaces           *runworkspace.Service
	TaskOps              *taskops.Service
}

type TaskRunner interface {
	Start(ctx context.Context, taskID string) error
	Cancel(ctx context.Context, taskID string) (bool, error)
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system", a.handleSystem)
	mux.HandleFunc("/api/tasks", a.handleTasks)
	mux.HandleFunc("/api/tasks/", a.handleTaskByID)
	mux.HandleFunc("/api/sessions/", a.handleSessionByKey)
	mux.HandleFunc("/api/acceptance", a.handleAcceptance)
	mux.HandleFunc("/api/mission-contract", a.handleMissionContract)
	mux.HandleFunc("/api/context", a.handleProjectContext)
	mux.HandleFunc("/api/secretary/messages", a.handleSecretaryMessages)
	mux.HandleFunc("/api/secretary/messages/stream", a.handleSecretaryMessagesStream)
	mux.HandleFunc("/api/secretary/clear", a.handleSecretaryClear)
	mux.HandleFunc("/api/audit/entries", a.handleAuditEntries)
	mux.HandleFunc("/api/audit/entries/", a.handleAuditEntryByID)
	mux.HandleFunc("/api/audit/sources", a.handleAuditSources)
	mux.HandleFunc("/api/audit/retention", a.handleAuditRetention)
	mux.HandleFunc("/api/templates", a.handlePromptTemplates)
	mux.HandleFunc("/api/templates/upsert", a.handlePromptTemplatesUpsert)
	mux.HandleFunc("/api/templates/delete", a.handlePromptTemplatesDelete)
	mux.HandleFunc("/api/tools", a.handleTools)
	mux.HandleFunc("/api/tools/status", a.handleToolsStatus)
	mux.HandleFunc("/api/tools/upsert", a.handleToolsUpsert)
	mux.HandleFunc("/api/tools/delete", a.handleToolsDelete)
	mux.HandleFunc("/api/providers", a.handleProviders)
	mux.HandleFunc("/api/providers/upsert", a.handleProvidersUpsert)
	mux.HandleFunc("/api/providers/delete", a.handleProvidersDelete)
	mux.HandleFunc("/api/providers/duplicate", a.handleProvidersDuplicate)
	mux.HandleFunc("/api/providers/reorder", a.handleProvidersReorder)
	mux.HandleFunc("/api/providers/activate", a.handleProvidersActivate)
	mux.HandleFunc("/api/providers/speedtest", a.handleProvidersSpeedTest)
	mux.HandleFunc("/api/providers/ping", a.handleProvidersPing)
	mux.HandleFunc("/api/providers/import", a.handleProvidersImport)
	mux.HandleFunc("/api/providers/import/live", a.handleProvidersImportLive)
	mux.HandleFunc("/api/providers/import/env", a.handleProvidersImportEnv)
	mux.HandleFunc("/api/providers/export", a.handleProvidersExport)
	mux.HandleFunc("/api/skills", a.handleSkills)
	mux.HandleFunc("/api/skills/link", a.handleSkillsLink)
	mux.HandleFunc("/api/skills/unlink", a.handleSkillsUnlink)
	mux.HandleFunc("/api/skills/sync", a.handleSkillsSync)
	mux.HandleFunc("/api/skills/tools", a.handleSkillsTools)
	mux.HandleFunc("/api/skills/onboarding", a.handleSkillsOnboarding)
	mux.HandleFunc("/api/skills/import", a.handleSkillsImportExisting)
	mux.HandleFunc("/api/skills/install/local", a.handleSkillsInstallLocal)
	mux.HandleFunc("/api/skills/git/candidates", a.handleSkillsGitCandidates)
	mux.HandleFunc("/api/skills/install/git", a.handleSkillsInstallGit)
	mux.HandleFunc("/api/skills/install/git/batch", a.handleSkillsInstallGitBatch)
	mux.HandleFunc("/api/skills/update", a.handleSkillsUpdate)
	mux.HandleFunc("/api/skills/versions", a.handleSkillVersions)
	mux.HandleFunc("/api/skills/versions/create", a.handleSkillVersionsCreate)
	mux.HandleFunc("/api/skills/versions/delete", a.handleSkillVersionsDelete)
	mux.HandleFunc("/api/skills/", a.handleSkillScopedRoutes)
	mux.HandleFunc("/api/fs/roots", a.handleFSRoots)
	mux.HandleFunc("/api/fs/list", a.handleFSList)
	mux.HandleFunc("/api/fs/read", a.handleFSRead)
	mux.HandleFunc("/api/fs/entries", a.handleFSEntries)
	mux.HandleFunc("/api/fs/write", a.handleFSWrite)
	mux.HandleFunc("/api/fs/mkdir", a.handleFSMkdir)
	mux.HandleFunc("/api/fs/delete", a.handleFSDelete)
	mux.HandleFunc("/api/events", a.handleEvents)
	mux.HandleFunc("/api/auth", a.handleAuth)
	mux.HandleFunc("/api/auth/status", a.handleAuthStatus)
	mux.HandleFunc("/api/auth/import/env", a.handleAuthImportEnv)
	return mux
}

func (a *API) handleProjectContext(w http.ResponseWriter, r *http.Request) {
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		ctx, ok, err := a.Tasks.GetProjectContext(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type resp struct {
			Content   string     `json:"content"`
			UpdatedAt *time.Time `json:"updated_at,omitempty"`
		}
		out := resp{Content: strings.TrimSpace(ctx.Content)}
		if ok && !ctx.UpdatedAt.IsZero() {
			v := ctx.UpdatedAt
			out.UpdatedAt = &v
		}
		writeJSON(w, out)
	case http.MethodPost:
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if _, err := a.Tasks.SetProjectContext(r.Context(), body.Content); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handlePromptTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	templates, err := a.Tasks.ListPromptTemplates(r.Context(), kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"templates": templates})
}

func (a *API) handlePromptTemplatesUpsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tpl, err := a.Tasks.UpsertPromptTemplate(r.Context(), tasks.UpsertPromptTemplateInput{
		ID:      body.ID,
		Title:   body.Title,
		Kind:    body.Kind,
		Content: body.Content,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"template": tpl})
}

func (a *API) handlePromptTemplatesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := a.Tasks.DeletePromptTemplate(r.Context(), body.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleAcceptance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}

	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		taskID := strings.TrimSpace(r.URL.Query().Get("task_id"))
		if taskID == "" {
			taskID = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if taskID != "" {
			t, err := a.Tasks.GetTask(r.Context(), taskID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			key = tasks.SessionKeyForTask(t)
		}
	}
	if key == "" {
		http.Error(w, "key or task_id is required", http.StatusBadRequest)
		return
	}

	state, ok, err := a.Tasks.GetAcceptanceState(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "state": nil})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "state": state})
}

func (a *API) handleMissionContract(w http.ResponseWriter, r *http.Request) {
	if a.Tasks == nil {
		http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
		return
	}

	resolveKey := func(rawKey, rawTaskID, rawID string) (string, error) {
		key := strings.TrimSpace(rawKey)
		if key != "" {
			return key, nil
		}
		taskID := strings.TrimSpace(rawTaskID)
		if taskID == "" {
			taskID = strings.TrimSpace(rawID)
		}
		if taskID == "" {
			return "", nil
		}
		t, err := a.Tasks.GetTask(r.Context(), taskID)
		if err != nil {
			return "", err
		}
		return tasks.SessionKeyForTask(t), nil
	}

	switch r.Method {
	case http.MethodGet:
		key, err := resolveKey(
			r.URL.Query().Get("key"),
			r.URL.Query().Get("task_id"),
			r.URL.Query().Get("id"),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if key == "" {
			http.Error(w, "key or task_id is required", http.StatusBadRequest)
			return
		}
		contract, ok, err := a.Tasks.GetMissionContract(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			writeJSON(w, map[string]any{"ok": false, "contract": nil})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "contract": contract})
	case http.MethodPost:
		var body struct {
			Key                string   `json:"key"`
			TaskID             string   `json:"task_id,omitempty"`
			ID                 string   `json:"id,omitempty"`
			Goal               string   `json:"goal"`
			Constraints        []string `json:"constraints"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			NonGoals           []string `json:"non_goals"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		key, err := resolveKey(body.Key, body.TaskID, body.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if key == "" {
			http.Error(w, "key or task_id is required", http.StatusBadRequest)
			return
		}
		contract, err := a.Tasks.UpsertMissionContract(r.Context(), tasks.UpsertMissionContractInput{
			Key:                key,
			Goal:               body.Goal,
			Constraints:        body.Constraints,
			AcceptanceCriteria: body.AcceptanceCriteria,
			NonGoals:           body.NonGoals,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "contract": contract})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, systeminfo.Snapshot())
}

func (a *API) handleFSRoots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	roots := a.FSRoots
	if len(roots) == 0 {
		roots = DefaultFSRoots()
	}
	writeJSON(w, map[string]any{"roots": roots})
}

func (a *API) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	path, err := resolveFSPath(raw, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.FSRoots
	if len(roots) == 0 {
		roots = DefaultFSRoots()
	}

	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	resp, err := ListDirs(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, resp)
}

func (a *API) handleFSRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	baseRaw := strings.TrimSpace(r.URL.Query().Get("base"))
	path, err := resolveFSPath(raw, baseRaw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.FSRoots
	if len(roots) == 0 {
		roots = DefaultFSRoots()
	}
	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if info.IsDir() {
		http.Error(w, "fs: not a file", http.StatusBadRequest)
		return
	}

	const maxBytes = 1 << 20 // 1 MiB
	limited := io.LimitReader(f, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	truncated := false
	if int64(len(data)) > maxBytes {
		truncated = true
		data = data[:maxBytes]
	}

	writeJSON(w, map[string]any{
		"path":      path,
		"size":      info.Size(),
		"truncated": truncated,
		"content":   string(data),
	})
}

func (a *API) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := parseInt(r.URL.Query().Get("limit"), 100)
		includeDeleted := strings.TrimSpace(r.URL.Query().Get("include_deleted")) == "1"
		items, err := a.Tasks.ListTasksWithOptions(r.Context(), limit, tasks.ListTasksOptions{
			IncludeDeleted: includeDeleted,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"tasks": items})
	case http.MethodPost:
		var in tasks.CreateTaskInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeTaskMutationInvalidJSON(w)
			return
		}
		if a.Tasks == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		if k := strings.TrimSpace(r.Header.Get("Idempotency-Key")); k != "" && strings.TrimSpace(in.IdempotencyKey) == "" {
			in.IdempotencyKey = k
		}
		ops := a.taskOpsOrShim()
		if ops == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		task, err := ops.CreateTask(r.Context(), in)
		if err != nil {
			writeTaskMutationProblem(w, err)
			return
		}
		writeTaskMutationResult(w, http.StatusOK, taskops.NewTaskMutationResult(taskops.ActionTaskCreate, task))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleSessionByKey(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	key := parts[0]
	if key == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "continue":
		a.handleSessionContinue(w, r, key)
		return
	case "preempt-continue":
		a.handleSessionPreemptContinue(w, r, key)
		return
	case "queue":
		a.handleSessionContinueQueue(w, r, key)
		return
	case "next-action":
		if len(parts) == 2 {
			a.handleSessionNextAction(w, r, key)
			return
		}
		if len(parts) == 3 && strings.TrimSpace(parts[2]) == "execute" {
			a.handleSessionNextActionExecute(w, r, key)
			return
		}
		http.NotFound(w, r)
		return
	case "workspace":
		if a.Tasks == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		if a.Workspaces == nil {
			http.Error(w, "workspaces not configured", http.StatusServiceUnavailable)
			return
		}

		conversationID, err := resolveConversationIDForSessionKey(r.Context(), a.Tasks, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		canonicalKey := tasks.ConversationKey(conversationID)
		if canonicalKey == "" {
			http.Error(w, "conversation_id is required", http.StatusBadRequest)
			return
		}

		if len(parts) == 2 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ws, ok, err := a.Workspaces.Get(r.Context(), canonicalKey)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				writeJSON(w, map[string]any{"ok": false, "workspace": nil})
				return
			}
			writeJSON(w, map[string]any{"ok": true, "workspace": ws})
			return
		}

		switch parts[2] {
		case "ensure":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			runs, err := a.Tasks.ListTasksByConversationID(r.Context(), conversationID, 1, tasks.ListTasksOptions{IncludeDeleted: true})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(runs) == 0 {
				http.Error(w, "session not found", http.StatusNotFound)
				return
			}
			latest := runs[0]
			ens, err := a.Workspaces.EnsureForTask(r.Context(), tasks.Task{
				ID:             strings.TrimSpace(latest.ID),
				ConversationID: conversationID,
				WorkDir:        strings.TrimSpace(latest.WorkDir),
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"ok":        true,
				"workspace": ens.Workspace,
				"logs":      ens.Logs,
			})
		case "merge":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			res, err := a.Workspaces.Merge(r.Context(), canonicalKey)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, res)
		case "discard":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := a.Workspaces.Discard(r.Context(), canonicalKey); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	case "rename":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if err := a.Tasks.RenameSession(r.Context(), key, body.Title); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case "delete":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := a.Tasks.DeleteSession(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func resolveConversationIDForSessionKey(ctx context.Context, store *tasks.Store, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("session key is required")
	}
	if store == nil {
		return "", errors.New("tasks store not configured")
	}
	if strings.HasPrefix(key, "c:") {
		cid := strings.TrimSpace(strings.TrimPrefix(key, "c:"))
		if cid == "" {
			return "", errors.New("conversation_id is required")
		}
		return cid, nil
	}
	if strings.HasPrefix(key, "t:") {
		taskID := strings.TrimSpace(strings.TrimPrefix(key, "t:"))
		if taskID == "" {
			return "", errors.New("task_id is required")
		}
		t, err := store.GetTask(ctx, taskID)
		if err != nil {
			return "", fmt.Errorf("task not found: %w", err)
		}
		if cid := strings.TrimSpace(t.ConversationID); cid != "" {
			return cid, nil
		}
		return strings.TrimSpace(t.ID), nil
	}
	if strings.HasPrefix(key, "s:") {
		sid := strings.TrimSpace(strings.TrimPrefix(key, "s:"))
		if sid == "" {
			return "", errors.New("session_id is required")
		}
		if cid, ok, err := store.ConversationIDForSessionID(ctx, sid); err != nil {
			return "", err
		} else if ok {
			return cid, nil
		}
		return "", errors.New("session not found")
	}
	return "", errors.New("invalid session key (expected c:/s:/t:)")
}

func shouldContinueViaRehydrate(t tasks.Task) bool {
	if t.Mode != tasks.ModeResume {
		return false
	}
	if isNoConversationFound(t.Warning) {
		return true
	}
	if isNoConversationFound(t.Error) {
		return true
	}
	return false
}

func isNoConversationFound(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "no conversation found") {
		return false
	}
	return strings.Contains(lower, "session")
}

func (a *API) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		task, err := a.Tasks.GetTask(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, task)
		return
	}

	switch parts[1] {
	case "cancel":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ok := false
		if a.Workers != nil {
			var err error
			ok, err = a.Workers.Cancel(r.Context(), id)
			if err != nil {
				writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
					"error":   "runner_unavailable",
					"message": err.Error(),
					"hint":    "restart the runner daemon (controlccx-runnerd)",
					"task_id": id,
				})
				return
			}
		}
		writeJSON(w, map[string]any{"ok": ok})
	case "approvals":
		if a.Tasks == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		// GET /api/tasks/{id}/approvals?status=pending
		if len(parts) == 2 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			rawStatus := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
			var status tasks.ApprovalStatus
			switch rawStatus {
			case "":
				status = ""
			case string(tasks.ApprovalStatusPending):
				status = tasks.ApprovalStatusPending
			case string(tasks.ApprovalStatusApproved):
				status = tasks.ApprovalStatusApproved
			case string(tasks.ApprovalStatusDenied):
				status = tasks.ApprovalStatusDenied
			case string(tasks.ApprovalStatusExpired):
				status = tasks.ApprovalStatusExpired
			default:
				http.Error(w, "invalid status", http.StatusBadRequest)
				return
			}
			list, err := a.Tasks.ListApprovalRequestsByTask(r.Context(), id, tasks.ListApprovalRequestsOptions{
				Status: status,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"approvals": list})
			return
		}

		// POST /api/tasks/{id}/approvals/{approval_id}/decision
		if len(parts) == 4 && strings.TrimSpace(parts[3]) == "decision" {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			approvalID := strings.TrimSpace(parts[2])
			if approvalID == "" {
				http.NotFound(w, r)
				return
			}
			var body struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			decision := strings.ToLower(strings.TrimSpace(body.Decision))
			switch decision {
			case "approve", "deny":
				// ok
			default:
				http.Error(w, "invalid decision", http.StatusBadRequest)
				return
			}
			if a.TaskOps != nil {
				if !supportsApprovalDecisionForwarder(a.TaskOps.Workers) {
					writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
						"error":   "runner_unavailable",
						"message": "runner does not support approvals",
						"hint":    "restart the runner daemon (controlccx-runnerd)",
						"task_id": id,
					})
					return
				}
				_, err := a.TaskOps.DecideApproval(r.Context(), id, approvalID, decision, strings.TrimSpace(body.Reason))
				if err != nil {
					writeApprovalDecisionError(w, id, approvalID, err)
					return
				}
				writeJSON(w, map[string]any{"ok": true})
				return
			}
			ar, ok, err := a.Tasks.GetApprovalRequest(r.Context(), approvalID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !ok || strings.TrimSpace(ar.TaskID) != strings.TrimSpace(id) {
				http.NotFound(w, r)
				return
			}
			if ar.Status != tasks.ApprovalStatusPending {
				writeJSONStatus(w, http.StatusConflict, map[string]any{
					"error":       "approval_not_pending",
					"message":     "approval already decided",
					"approval_id": approvalID,
					"status":      ar.Status,
				})
				return
			}
			forwarder, ok := a.Workers.(interface {
				SubmitApprovalDecision(ctx context.Context, taskID string, approvalID string, decision string, reason string) error
			})
			if !ok || forwarder == nil {
				writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
					"error":   "runner_unavailable",
					"message": "runner does not support approvals",
					"hint":    "restart the runner daemon (controlccx-runnerd)",
					"task_id": id,
				})
				return
			}
			if err := forwarder.SubmitApprovalDecision(r.Context(), id, approvalID, decision, strings.TrimSpace(body.Reason)); err != nil {
				writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
					"error":       "runner_unavailable",
					"message":     err.Error(),
					"hint":        "restart the runner daemon (controlccx-runnerd)",
					"task_id":     id,
					"approval_id": approvalID,
				})
				return
			}
			writeJSON(w, map[string]any{"ok": true})
			return
		}

		http.NotFound(w, r)
	case "logs":
		if len(parts) >= 3 && parts[2] == "export" {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			streams := parseStreams(r.URL.Query().Get("streams"))
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			logs, err := a.Tasks.ListAllLogsFiltered(r.Context(), id, tasks.ListLogsFilter{
				Streams: streams,
				Query:   q,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "logs-"+id+".txt"))
			for _, l := range logs {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", l.Time.Format(time.RFC3339), l.Stream, l.Message)
			}
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		after := parseInt64(r.URL.Query().Get("after"), 0)
		limit := parseInt(r.URL.Query().Get("limit"), 500)
		streams := parseStreams(r.URL.Query().Get("streams"))
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		logs, err := a.Tasks.ListLogsFiltered(r.Context(), id, after, limit, tasks.ListLogsFilter{
			Streams: streams,
			Query:   q,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"logs": logs})
	case "trace":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		task, err := a.Tasks.GetTask(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		inv, ok, err := a.Tasks.GetInvocation(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			writeJSON(w, map[string]any{"task": task, "invocation": nil})
			return
		}
		writeJSON(w, map[string]any{"task": task, "invocation": inv})
	case "resume":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Prompt                string            `json:"prompt"`
			UnsafeAutomation      bool              `json:"unsafe_automation,omitempty"`
			SafetyEnvelope        string            `json:"safety_envelope,omitempty"`
			SafetyPreset          string            `json:"safety_preset,omitempty"`
			TaskIntent            string            `json:"task_intent,omitempty"`
			NetworkTier           tasks.NetworkTier `json:"network_tier,omitempty"`
			CodexSandbox          string            `json:"codex_sandbox,omitempty"`
			CodexApprovalPolicy   string            `json:"codex_approval_policy,omitempty"`
			CodexSearch           bool              `json:"codex_search,omitempty"`
			ClaudePermissionMode  string            `json:"claude_permission_mode,omitempty"`
			ClaudeSandbox         bool              `json:"claude_sandbox,omitempty"`
			ClaudeWebFetchDomains []string          `json:"claude_webfetch_domains,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeTaskMutationInvalidJSON(w)
			return
		}
		ops := a.taskOpsOrShim()
		if ops == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		t, err := ops.ResumeTask(r.Context(), id, taskops.RunOptions{
			Prompt:                body.Prompt,
			UnsafeAutomation:      body.UnsafeAutomation,
			SafetyEnvelope:        body.SafetyEnvelope,
			SafetyPreset:          body.SafetyPreset,
			TaskIntent:            body.TaskIntent,
			NetworkTier:           body.NetworkTier,
			CodexSandbox:          body.CodexSandbox,
			CodexApprovalPolicy:   body.CodexApprovalPolicy,
			CodexSearch:           body.CodexSearch,
			ClaudePermissionMode:  body.ClaudePermissionMode,
			ClaudeSandbox:         body.ClaudeSandbox,
			ClaudeWebFetchDomains: body.ClaudeWebFetchDomains,
		})
		if err != nil {
			writeTaskMutationProblem(w, err)
			return
		}
		writeTaskMutationResult(w, http.StatusOK, taskops.NewTaskMutationResult(taskops.ActionTaskResume, t))
	case "enter-unsafe":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a.Tasks == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Prompt string `json:"prompt,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeTaskMutationInvalidJSON(w)
			return
		}
		ops := a.taskOpsOrShim()
		if ops == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		t, err := ops.EnterUnsafeTask(r.Context(), id, strings.TrimSpace(body.Prompt))
		if err != nil {
			writeTaskMutationProblem(w, err)
			return
		}
		writeTaskMutationResult(w, http.StatusOK, taskops.NewTaskMutationResult(taskops.ActionTaskEnterUnsafe, t))
	case "rehydrate":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Prompt                string            `json:"prompt"`
			UnsafeAutomation      bool              `json:"unsafe_automation,omitempty"`
			SafetyEnvelope        string            `json:"safety_envelope,omitempty"`
			SafetyPreset          string            `json:"safety_preset,omitempty"`
			TaskIntent            string            `json:"task_intent,omitempty"`
			NetworkTier           tasks.NetworkTier `json:"network_tier,omitempty"`
			CodexSandbox          string            `json:"codex_sandbox,omitempty"`
			CodexApprovalPolicy   string            `json:"codex_approval_policy,omitempty"`
			CodexSearch           bool              `json:"codex_search,omitempty"`
			ClaudePermissionMode  string            `json:"claude_permission_mode,omitempty"`
			ClaudeSandbox         bool              `json:"claude_sandbox,omitempty"`
			ClaudeWebFetchDomains []string          `json:"claude_webfetch_domains,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeTaskMutationInvalidJSON(w)
			return
		}
		ops := a.taskOpsOrShim()
		if ops == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}
		t, err := ops.RehydrateTask(r.Context(), id, taskops.RunOptions{
			Prompt:                body.Prompt,
			UnsafeAutomation:      body.UnsafeAutomation,
			SafetyEnvelope:        body.SafetyEnvelope,
			SafetyPreset:          body.SafetyPreset,
			TaskIntent:            body.TaskIntent,
			NetworkTier:           body.NetworkTier,
			CodexSandbox:          body.CodexSandbox,
			CodexApprovalPolicy:   body.CodexApprovalPolicy,
			CodexSearch:           body.CodexSearch,
			ClaudePermissionMode:  body.ClaudePermissionMode,
			ClaudeSandbox:         body.ClaudeSandbox,
			ClaudeWebFetchDomains: body.ClaudeWebFetchDomains,
		})
		if err != nil {
			writeTaskMutationProblem(w, err)
			return
		}
		writeTaskMutationResult(w, http.StatusOK, taskops.NewTaskMutationResult(taskops.ActionTaskRehydrate, t))
	default:
		http.NotFound(w, r)
	}
}

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	events.ServeSSE(a.Hub)(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseInt64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func parseStreams(raw string) []tasks.LogStream {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	allowed := map[string]tasks.LogStream{
		"stdout":    tasks.LogStdout,
		"stderr":    tasks.LogStderr,
		"system":    tasks.LogSystem,
		"assistant": tasks.LogAssistant,
	}
	var out []tasks.LogStream
	seen := map[tasks.LogStream]bool{}
	for _, part := range strings.Split(raw, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		st, ok := allowed[key]
		if !ok || seen[st] {
			continue
		}
		seen[st] = true
		out = append(out, st)
	}
	return out
}

func supportsApprovalDecisionForwarder(runner any) bool {
	if runner == nil {
		return false
	}
	fw, ok := runner.(interface {
		SubmitApprovalDecision(ctx context.Context, taskID string, approvalID string, decision string, reason string) error
	})
	return ok && fw != nil
}

func writeApprovalDecisionError(w http.ResponseWriter, taskID string, approvalID string, err error) {
	var notPending *tasks.ApprovalNotPendingError
	if errors.As(err, &notPending) {
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"error":       "approval_not_pending",
			"message":     notPending.Error(),
			"approval_id": strings.TrimSpace(approvalID),
			"status":      strings.TrimSpace(string(notPending.Status)),
		})
		return
	}
	var runnerErr *taskops.RunnerUnavailableError
	if errors.As(err, &runnerErr) {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "runner_unavailable",
			"message": err.Error(),
			"hint":    "restart the runner daemon (controlccx-runnerd)",
			"task_id": taskID,
		})
		return
	}
	msg := strings.TrimSpace(err.Error())
	switch msg {
	case "approval not found":
		http.Error(w, msg, http.StatusNotFound)
	case "no pending approval found":
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"error":   "approval_not_pending",
			"message": msg,
			"task_id": taskID,
		})
	default:
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func writeTaskMutationError(w http.ResponseWriter, err error) {
	writeTaskMutationProblem(w, err)
}

func writeTaskMutationInvalidJSON(w http.ResponseWriter) {
	writeTaskMutationProblem(w, errors.New("invalid json"))
}

func writeTaskMutationResult(w http.ResponseWriter, status int, result taskops.MutationResult) {
	if status <= 0 {
		status = http.StatusOK
	}
	result.OK = true
	writeJSONStatus(w, status, result)
}

func writeTaskMutationProblem(w http.ResponseWriter, err error) {
	problem := taskops.ParseMutationError(err)
	status := problem.Status
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	writeJSONStatus(w, status, problem)
}

func (a *API) taskOpsOrShim() *taskops.Service {
	if a == nil {
		return nil
	}
	if a.TaskOps != nil {
		return a.TaskOps
	}
	if a.Tasks == nil {
		return nil
	}
	return &taskops.Service{
		Tasks:   a.Tasks,
		Workers: a.Workers,
		Hub:     a.Hub,
		Tools:   a.Tools,
	}
}

func (a *API) validate() error {
	if a.Tasks == nil {
		return errors.New("api: missing dependencies")
	}
	return nil
}
