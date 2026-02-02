package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/runsafe"
	"controlccx/internal/runworkspace"
	"controlccx/internal/skills"
	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
	"controlccx/internal/worker"
)

type API struct {
	Tasks                *tasks.Store
	Workers              *worker.Manager
	Observer             *observer.Service
	Chat                 *chat.Store
	Hub                  *events.Hub
	FSRoots              []FSRoot
	Auth                 *auth.Store
	Skills               *skills.Service
	SkillVersions        *skills.VersionsService
	SkillVersionsBySkill *skills.PerSkillVersionsService
	Tools                *tooling.Service
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system", a.handleSystem)
	mux.HandleFunc("/api/tasks", a.handleTasks)
	mux.HandleFunc("/api/tasks/", a.handleTaskByID)
	mux.HandleFunc("/api/sessions/", a.handleSessionByKey)
	mux.HandleFunc("/api/acceptance", a.handleAcceptance)
	mux.HandleFunc("/api/tools", a.handleTools)
	mux.HandleFunc("/api/tools/upsert", a.handleToolsUpsert)
	mux.HandleFunc("/api/tools/delete", a.handleToolsDelete)
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
	mux.HandleFunc("/api/chat", a.handleChat)
	mux.HandleFunc("/api/auth", a.handleAuth)
	mux.HandleFunc("/api/auth/status", a.handleAuthStatus)
	return mux
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

	path := filepath.Clean(raw)
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			http.Error(w, "cannot resolve cwd", http.StatusInternalServerError)
			return
		}
		path = filepath.Join(cwd, path)
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

	path := filepath.Clean(raw)
	if baseRaw != "" && !filepath.IsAbs(path) {
		base := filepath.Clean(baseRaw)
		if !filepath.IsAbs(base) {
			cwd, err := os.Getwd()
			if err != nil {
				http.Error(w, "cannot resolve cwd", http.StatusInternalServerError)
				return
			}
			base = filepath.Join(cwd, base)
		}
		path = filepath.Join(base, path)
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			http.Error(w, "cannot resolve cwd", http.StatusInternalServerError)
			return
		}
		path = filepath.Join(cwd, path)
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
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if a.Tools != nil {
			if _, ok := a.Tools.Resolve(string(in.WorkerType)); !ok {
				http.Error(w, "unknown tool id: "+string(in.WorkerType), http.StatusBadRequest)
				return
			}
		}

		// Run Safety Autopilot: fill safety options when the client did not explicitly set them.
		driver := in.WorkerType
		if a.Tools != nil {
			if profile, ok := a.Tools.Resolve(string(in.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
		llm := runsafe.LLMBackend(nil)
		if a.Observer != nil {
			llm = a.Observer.LLM
		}
		in, ap := runsafe.ApplyAutopilot(r.Context(), in, runsafe.ApplyOptions{
			Driver:   driver,
			Envelope: envelope,
			Classify: runsafe.ClassifyOptions{LLM: llm},
		})

		task, err := a.Tasks.CreateTask(r.Context(), in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
				_, _ = a.Tasks.AppendLog(r.Context(), task.ID, tasks.LogSystem, audit)
			}
		}

		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: task})
		}
		if a.Workers != nil {
			_ = a.Workers.Start(r.Context(), task.ID)
		}
		writeJSON(w, task)
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
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a.Tasks == nil {
			http.Error(w, "tasks store not configured", http.StatusServiceUnavailable)
			return
		}

		var body struct {
			Prompt                string   `json:"prompt"`
			UnsafeAutomation      bool     `json:"unsafe_automation,omitempty"`
			SafetyEnvelope        string   `json:"safety_envelope,omitempty"`
			SafetyPreset          string   `json:"safety_preset,omitempty"`
			TaskIntent            string   `json:"task_intent,omitempty"`
			CodexSandbox          string   `json:"codex_sandbox,omitempty"`
			CodexApprovalPolicy   string   `json:"codex_approval_policy,omitempty"`
			CodexSearch           bool     `json:"codex_search,omitempty"`
			ClaudePermissionMode  string   `json:"claude_permission_mode,omitempty"`
			ClaudeSandbox         bool     `json:"claude_sandbox,omitempty"`
			ClaudeWebFetchDomains []string `json:"claude_webfetch_domains,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		conversationID, err := resolveConversationIDForSessionKey(r.Context(), a.Tasks, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		runs, err := a.Tasks.ListTasksByConversationID(r.Context(), conversationID, 500, tasks.ListTasksOptions{IncludeDeleted: true})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(runs) == 0 {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		latest := runs[0]
		if latest.SessionDeletedAt != nil {
			http.Error(w, "session is deleted; cannot continue", http.StatusBadRequest)
			return
		}

		for _, t := range runs {
			if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued {
				http.Error(w, "session already has a running task (task_id="+t.ID+" status="+string(t.Status)+")", http.StatusConflict)
				return
			}
		}

		if latest.Status == tasks.StatusBlocked {
			http.Error(w, "当前会话存在被阻塞的 run（需要人工确认/放权）。请先处理阻塞或选择高风险继续。", http.StatusConflict)
			return
		}

		if shouldContinueViaRehydrate(latest) {
			driver := latest.WorkerType
			if a.Tools != nil {
				if profile, ok := a.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
					driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
				}
			}
			if driver != tasks.WorkerClaudeCode {
				http.Error(w, "rehydrate is only supported for claude-code sessions", http.StatusBadRequest)
				return
			}

			ws, ok, err := resolveSessionWorkspaceForConversation(r.Context(), a.Tasks, tasks.ConversationKey(conversationID), runs)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if ok && ws.Status == tasks.WorkspaceStatusActive {
				http.Error(w, "无法新建会话继续：该会话仍有隔离工作区处于 active。请先在 Workspace 面板执行 Merge（把改动合并回 base_workdir）后再重试。", http.StatusConflict)
				return
			}

			prompt := strings.TrimSpace(body.Prompt)
			if prompt == "" {
				prompt = "continue"
			}
			ctxPrompt, err := tasks.BuildRehydratePrompt(r.Context(), a.Tasks, conversationID, prompt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// SafetyEnvelope is an autopilot hint (UI-level “one-time unlock”); it does not count as an explicit safety override.
			explicitSafety := body.UnsafeAutomation ||
				strings.TrimSpace(body.SafetyPreset) != "" ||
				strings.TrimSpace(body.TaskIntent) != "" ||
				strings.TrimSpace(body.CodexSandbox) != "" ||
				strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
				body.CodexSearch ||
				strings.TrimSpace(body.ClaudePermissionMode) != "" ||
				body.ClaudeSandbox ||
				len(body.ClaudeWebFetchDomains) > 0

			unsafe := body.UnsafeAutomation
			safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
			safetyPreset := strings.TrimSpace(body.SafetyPreset)
			taskIntent := strings.TrimSpace(body.TaskIntent)
			codexSandbox := strings.TrimSpace(body.CodexSandbox)
			codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
			codexSearch := body.CodexSearch
			claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
			claudeSandbox := body.ClaudeSandbox
			claudeDomains := body.ClaudeWebFetchDomains

			if !explicitSafety {
				unsafe = latest.UnsafeAutomation
				safetyPreset = strings.TrimSpace(latest.SafetyPreset)
				taskIntent = strings.TrimSpace(latest.TaskIntent)
				codexSandbox = strings.TrimSpace(latest.CodexSandbox)
				codexApprovalPolicy = strings.TrimSpace(latest.CodexApprovalPolicy)
				codexSearch = latest.CodexSearch
				claudePermissionMode = strings.TrimSpace(latest.ClaudePermissionMode)
				claudeSandbox = latest.ClaudeSandbox
				claudeDomains = append([]string{}, latest.ClaudeWebFetchDomains...)
			}

			in := tasks.CreateTaskInput{
				WorkerType:            latest.WorkerType,
				Mode:                  tasks.ModeNew,
				ConversationID:        conversationID,
				UnsafeAutomation:      unsafe,
				SafetyEnvelope:        safetyEnvelope,
				SafetyPreset:          safetyPreset,
				TaskIntent:            taskIntent,
				CodexSandbox:          codexSandbox,
				CodexApprovalPolicy:   codexApprovalPolicy,
				CodexSearch:           codexSearch,
				ClaudePermissionMode:  claudePermissionMode,
				ClaudeSandbox:         claudeSandbox,
				ClaudeWebFetchDomains: claudeDomains,
				Prompt:                ctxPrompt,
				WorkDir:               latest.WorkDir,
				SessionID:             "",
			}

			envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
			llm := runsafe.LLMBackend(nil)
			if a.Observer != nil {
				llm = a.Observer.LLM
			}
			in, ap := runsafe.ApplyAutopilot(r.Context(), in, runsafe.ApplyOptions{
				Driver:   driver,
				Envelope: envelope,
				Classify: runsafe.ClassifyOptions{LLM: llm},
			})

			newTask, err := a.Tasks.CreateTask(r.Context(), in)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", latest.ID, strings.TrimSpace(latest.SessionID)))

			if ap.Applied {
				if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
					_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, audit)
				}
			}

			if a.Hub != nil {
				a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
			}
			if a.Workers != nil {
				_ = a.Workers.Start(r.Context(), newTask.ID)
			}
			writeJSON(w, newTask)
			return
		}

		if strings.TrimSpace(latest.SessionID) == "" {
			http.Error(w, "task has no session_id to resume", http.StatusBadRequest)
			return
		}

		// Ensure resume uses the same run workspace as the session origin when available.
		desiredWSKey := tasks.SessionKeyForTask(latest)
		legacyWSKey := tasks.SessionKey(latest.ID, "")
		legacySessionKey := tasks.SessionKey("", latest.SessionID)
		if _, ok, err := a.Tasks.GetSessionWorkspace(r.Context(), desiredWSKey); err == nil && !ok {
			if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacySessionKey); err == nil && ok2 {
				_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacySessionKey, desiredWSKey)
			} else if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyWSKey); err == nil && ok2 {
				_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyWSKey, desiredWSKey)
			}
		}

		// SafetyEnvelope is an autopilot hint (UI-level “one-time unlock”); it does not count as an explicit safety override.
		explicitSafety := body.UnsafeAutomation ||
			strings.TrimSpace(body.SafetyPreset) != "" ||
			strings.TrimSpace(body.TaskIntent) != "" ||
			strings.TrimSpace(body.CodexSandbox) != "" ||
			strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
			body.CodexSearch ||
			strings.TrimSpace(body.ClaudePermissionMode) != "" ||
			body.ClaudeSandbox ||
			len(body.ClaudeWebFetchDomains) > 0

		unsafe := body.UnsafeAutomation
		safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
		safetyPreset := strings.TrimSpace(body.SafetyPreset)
		taskIntent := strings.TrimSpace(body.TaskIntent)
		codexSandbox := strings.TrimSpace(body.CodexSandbox)
		codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
		codexSearch := body.CodexSearch
		claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
		claudeSandbox := body.ClaudeSandbox
		claudeDomains := body.ClaudeWebFetchDomains

		if !explicitSafety {
			unsafe = latest.UnsafeAutomation
			safetyPreset = strings.TrimSpace(latest.SafetyPreset)
			taskIntent = strings.TrimSpace(latest.TaskIntent)
			codexSandbox = strings.TrimSpace(latest.CodexSandbox)
			codexApprovalPolicy = strings.TrimSpace(latest.CodexApprovalPolicy)
			codexSearch = latest.CodexSearch
			claudePermissionMode = strings.TrimSpace(latest.ClaudePermissionMode)
			claudeSandbox = latest.ClaudeSandbox
			claudeDomains = append([]string{}, latest.ClaudeWebFetchDomains...)
		}

		prompt := strings.TrimSpace(body.Prompt)
		if prompt == "" {
			prompt = "continue"
		}

		resumeIn := tasks.CreateTaskInput{
			WorkerType:            latest.WorkerType,
			Mode:                  tasks.ModeResume,
			ConversationID:        conversationID,
			UnsafeAutomation:      unsafe,
			SafetyEnvelope:        safetyEnvelope,
			SafetyPreset:          safetyPreset,
			TaskIntent:            taskIntent,
			CodexSandbox:          codexSandbox,
			CodexApprovalPolicy:   codexApprovalPolicy,
			CodexSearch:           codexSearch,
			ClaudePermissionMode:  claudePermissionMode,
			ClaudeSandbox:         claudeSandbox,
			ClaudeWebFetchDomains: claudeDomains,
			Prompt:                prompt,
			WorkDir:               latest.WorkDir,
			SessionID:             strings.TrimSpace(latest.SessionID),
			Warning:               latest.Warning,
		}

		driver := latest.WorkerType
		if a.Tools != nil {
			if profile, ok := a.Tools.Resolve(string(latest.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		envelope := runsafe.SafetyEnvelope(strings.TrimSpace(resumeIn.SafetyEnvelope))
		llm := runsafe.LLMBackend(nil)
		if a.Observer != nil {
			llm = a.Observer.LLM
		}
		resumeIn, ap := runsafe.ApplyAutopilot(r.Context(), resumeIn, runsafe.ApplyOptions{
			Driver:   driver,
			Envelope: envelope,
			Classify: runsafe.ClassifyOptions{LLM: llm},
		})

		newTask, err := a.Tasks.CreateTask(r.Context(), resumeIn)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, resumeIn, true); strings.TrimSpace(audit) != "" {
				_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, audit)
			}
		}
		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
		}
		if a.Workers != nil {
			_ = a.Workers.Start(r.Context(), newTask.ID)
		}
		writeJSON(w, newTask)
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

func resolveSessionWorkspaceForConversation(ctx context.Context, store *tasks.Store, desiredKey string, runs []tasks.Task) (tasks.SessionWorkspace, bool, error) {
	if store == nil {
		return tasks.SessionWorkspace{}, false, errors.New("tasks store not configured")
	}
	desiredKey = strings.TrimSpace(desiredKey)
	if desiredKey == "" {
		return tasks.SessionWorkspace{}, false, nil
	}

	if ws, ok, err := store.GetSessionWorkspace(ctx, desiredKey); err != nil {
		return tasks.SessionWorkspace{}, false, err
	} else if ok {
		return ws, true, nil
	}

	for _, t := range runs {
		if sid := strings.TrimSpace(t.SessionID); sid != "" {
			legacySessionKey := tasks.SessionKey("", sid)
			if legacySessionKey != desiredKey {
				if ws, ok, err := store.GetSessionWorkspace(ctx, legacySessionKey); err != nil {
					return tasks.SessionWorkspace{}, false, err
				} else if ok {
					_ = store.MigrateSessionWorkspaceKey(ctx, legacySessionKey, desiredKey)
					if migrated, ok2, err := store.GetSessionWorkspace(ctx, desiredKey); err == nil && ok2 {
						return migrated, true, nil
					}
					return ws, true, nil
				}
			}
		}
		legacyTaskKey := tasks.SessionKey(t.ID, "")
		if legacyTaskKey != desiredKey {
			if ws, ok, err := store.GetSessionWorkspace(ctx, legacyTaskKey); err != nil {
				return tasks.SessionWorkspace{}, false, err
			} else if ok {
				_ = store.MigrateSessionWorkspaceKey(ctx, legacyTaskKey, desiredKey)
				if migrated, ok2, err := store.GetSessionWorkspace(ctx, desiredKey); err == nil && ok2 {
					return migrated, true, nil
				}
				return ws, true, nil
			}
		}
	}

	return tasks.SessionWorkspace{}, false, nil
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
			ok = a.Workers.Cancel(id)
		}
		writeJSON(w, map[string]any{"ok": ok})
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
	case "workspace":
		task, err := a.Tasks.GetTask(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		desiredKey := tasks.SessionKeyForTask(task)
		key := desiredKey
		// Backward-compatible: older runs stored workspaces under session_id/task-id keys.
		// Prefer migrating them to the conversation-scoped key so the UI stays stable.
		if _, ok, err := a.Tasks.GetSessionWorkspace(r.Context(), desiredKey); err == nil && !ok {
			var fallbacks []string
			if strings.TrimSpace(task.SessionID) != "" {
				fallbacks = append(fallbacks, tasks.SessionKey("", task.SessionID))
			}
			fallbacks = append(fallbacks, tasks.SessionKey(task.ID, ""))

			for _, legacyKey := range fallbacks {
				if strings.TrimSpace(legacyKey) == "" || legacyKey == desiredKey {
					continue
				}
				if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyKey); err == nil && ok2 {
					_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyKey, desiredKey)
					if _, ok3, err := a.Tasks.GetSessionWorkspace(r.Context(), desiredKey); err == nil && ok3 {
						key = desiredKey
						break
					}
					// If migration didn't stick, still allow reading/merging the legacy mapping.
					key = legacyKey
					break
				}
			}
			if key == desiredKey && strings.TrimSpace(task.SessionID) != "" {
				all, err := a.Tasks.ListTasksWithOptions(r.Context(), 500, tasks.ListTasksOptions{IncludeDeleted: true})
				if err == nil {
					sid := strings.TrimSpace(task.SessionID)
					for _, t := range all {
						if strings.TrimSpace(t.SessionID) != sid {
							continue
						}
						legacyKey := tasks.SessionKey(t.ID, "")
						if strings.TrimSpace(legacyKey) == "" || legacyKey == desiredKey {
							continue
						}
						if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyKey); err == nil && ok2 {
							_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyKey, desiredKey)
							if _, ok3, err := a.Tasks.GetSessionWorkspace(r.Context(), desiredKey); err == nil && ok3 {
								key = desiredKey
							} else {
								key = legacyKey
							}
							break
						}
					}
				}
			}
		}
		if len(parts) == 2 {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			ws, ok, err := a.Tasks.GetSessionWorkspace(r.Context(), key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				writeJSON(w, map[string]any{"workspace": nil})
				return
			}
			writeJSON(w, map[string]any{"workspace": ws})
			return
		}

		if len(parts) < 3 {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		svc := runworkspace.NewService(a.Tasks)
		switch parts[2] {
		case "merge":
			if err := svc.Merge(r.Context(), key); err != nil {
				var conflict *runworkspace.ConflictError
				if errors.As(err, &conflict) {
					writeJSONStatus(w, http.StatusConflict, conflict)
					return
				}
				if strings.Contains(err.Error(), "workspace not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		case "discard":
			if err := svc.Discard(r.Context(), key); err != nil {
				if strings.Contains(err.Error(), "workspace not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	case "resume":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Prompt                string   `json:"prompt"`
			UnsafeAutomation      bool     `json:"unsafe_automation,omitempty"`
			SafetyEnvelope        string   `json:"safety_envelope,omitempty"`
			SafetyPreset          string   `json:"safety_preset,omitempty"`
			TaskIntent            string   `json:"task_intent,omitempty"`
			CodexSandbox          string   `json:"codex_sandbox,omitempty"`
			CodexApprovalPolicy   string   `json:"codex_approval_policy,omitempty"`
			CodexSearch           bool     `json:"codex_search,omitempty"`
			ClaudePermissionMode  string   `json:"claude_permission_mode,omitempty"`
			ClaudeSandbox         bool     `json:"claude_sandbox,omitempty"`
			ClaudeWebFetchDomains []string `json:"claude_webfetch_domains,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		prev, err := a.Tasks.GetTask(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if a.Tools != nil {
			if _, ok := a.Tools.Resolve(string(prev.WorkerType)); !ok {
				http.Error(w, "unknown tool id: "+string(prev.WorkerType), http.StatusBadRequest)
				return
			}
		}
		if strings.TrimSpace(prev.SessionID) == "" {
			http.Error(w, "task has no session_id to resume", http.StatusBadRequest)
			return
		}
		// Single-flight: avoid creating multiple overlapping resume runs for the same session.
		// This prevents "resume storms" (e.g. double-clicking Resume, autopilot+manual overlap).
		if a.Tasks != nil {
			sid := strings.TrimSpace(prev.SessionID)
			all, err := a.Tasks.ListTasksWithOptions(r.Context(), 500, tasks.ListTasksOptions{IncludeDeleted: true})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, t := range all {
				if t.ID == prev.ID {
					continue
				}
				if strings.TrimSpace(t.SessionID) != sid {
					continue
				}
				if t.Status == tasks.StatusRunning || t.Status == tasks.StatusQueued {
					http.Error(w, "session already has a running task (task_id="+t.ID+" status="+string(t.Status)+")", http.StatusConflict)
					return
				}
			}
		}

		// Ensure resume uses the same run workspace as the session origin when available.
		// Claude Code resumes are scoped to the project directory; changing directories
		// (e.g. new per-run workspaces) can make the session non-resumable.
		desiredWSKey := tasks.SessionKeyForTask(prev)
		legacyWSKey := tasks.SessionKey(prev.ID, "")
		legacySessionKey := tasks.SessionKey("", prev.SessionID)
		if _, ok, err := a.Tasks.GetSessionWorkspace(r.Context(), desiredWSKey); err == nil && !ok {
			if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacySessionKey); err == nil && ok2 {
				_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacySessionKey, desiredWSKey)
			} else if _, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyWSKey); err == nil && ok2 {
				_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyWSKey, desiredWSKey)
			}
		}

		// SafetyEnvelope is an autopilot hint (UI-level “one-time unlock”); it does not count as an explicit safety override.
		explicitSafety := body.UnsafeAutomation ||
			strings.TrimSpace(body.SafetyPreset) != "" ||
			strings.TrimSpace(body.TaskIntent) != "" ||
			strings.TrimSpace(body.CodexSandbox) != "" ||
			strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
			body.CodexSearch ||
			strings.TrimSpace(body.ClaudePermissionMode) != "" ||
			body.ClaudeSandbox ||
			len(body.ClaudeWebFetchDomains) > 0

		unsafe := body.UnsafeAutomation
		safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
		safetyPreset := strings.TrimSpace(body.SafetyPreset)
		taskIntent := strings.TrimSpace(body.TaskIntent)
		codexSandbox := strings.TrimSpace(body.CodexSandbox)
		codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
		codexSearch := body.CodexSearch
		claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
		claudeSandbox := body.ClaudeSandbox
		claudeDomains := body.ClaudeWebFetchDomains

		if !explicitSafety {
			unsafe = prev.UnsafeAutomation
			safetyPreset = strings.TrimSpace(prev.SafetyPreset)
			taskIntent = strings.TrimSpace(prev.TaskIntent)
			codexSandbox = strings.TrimSpace(prev.CodexSandbox)
			codexApprovalPolicy = strings.TrimSpace(prev.CodexApprovalPolicy)
			codexSearch = prev.CodexSearch
			claudePermissionMode = strings.TrimSpace(prev.ClaudePermissionMode)
			claudeSandbox = prev.ClaudeSandbox
			claudeDomains = append([]string{}, prev.ClaudeWebFetchDomains...)
		}

		resumeIn := tasks.CreateTaskInput{
			WorkerType:            prev.WorkerType,
			Mode:                  tasks.ModeResume,
			ConversationID:        prev.ConversationID,
			UnsafeAutomation:      unsafe,
			SafetyEnvelope:        safetyEnvelope,
			SafetyPreset:          safetyPreset,
			TaskIntent:            taskIntent,
			CodexSandbox:          codexSandbox,
			CodexApprovalPolicy:   codexApprovalPolicy,
			CodexSearch:           codexSearch,
			ClaudePermissionMode:  claudePermissionMode,
			ClaudeSandbox:         claudeSandbox,
			ClaudeWebFetchDomains: claudeDomains,
			Prompt:                body.Prompt,
			WorkDir:               prev.WorkDir,
			SessionID:             prev.SessionID,
			Warning:               prev.Warning,
		}

		driver := prev.WorkerType
		if a.Tools != nil {
			if profile, ok := a.Tools.Resolve(string(prev.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		envelope := runsafe.SafetyEnvelope(strings.TrimSpace(resumeIn.SafetyEnvelope))
		llm := runsafe.LLMBackend(nil)
		if a.Observer != nil {
			llm = a.Observer.LLM
		}
		resumeIn, ap := runsafe.ApplyAutopilot(r.Context(), resumeIn, runsafe.ApplyOptions{
			Driver:   driver,
			Envelope: envelope,
			Classify: runsafe.ClassifyOptions{LLM: llm},
		})

		newTask, err := a.Tasks.CreateTask(r.Context(), resumeIn)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, resumeIn, true); strings.TrimSpace(audit) != "" {
				_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, audit)
			}
		}
		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
		}
		if a.Workers != nil {
			_ = a.Workers.Start(r.Context(), newTask.ID)
		}
		writeJSON(w, newTask)
	case "rehydrate":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Prompt                string   `json:"prompt"`
			UnsafeAutomation      bool     `json:"unsafe_automation,omitempty"`
			SafetyEnvelope        string   `json:"safety_envelope,omitempty"`
			SafetyPreset          string   `json:"safety_preset,omitempty"`
			TaskIntent            string   `json:"task_intent,omitempty"`
			CodexSandbox          string   `json:"codex_sandbox,omitempty"`
			CodexApprovalPolicy   string   `json:"codex_approval_policy,omitempty"`
			CodexSearch           bool     `json:"codex_search,omitempty"`
			ClaudePermissionMode  string   `json:"claude_permission_mode,omitempty"`
			ClaudeSandbox         bool     `json:"claude_sandbox,omitempty"`
			ClaudeWebFetchDomains []string `json:"claude_webfetch_domains,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		src, err := a.Tasks.GetTask(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if a.Tools != nil {
			if _, ok := a.Tools.Resolve(string(src.WorkerType)); !ok {
				http.Error(w, "unknown tool id: "+string(src.WorkerType), http.StatusBadRequest)
				return
			}
		}
		if strings.TrimSpace(src.SessionID) == "" {
			http.Error(w, "task has no session_id to rehydrate", http.StatusBadRequest)
			return
		}

		// Currently, only Claude Code supports resume-missing-session recovery.
		driver := src.WorkerType
		if a.Tools != nil {
			if profile, ok := a.Tools.Resolve(string(src.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
				driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
			}
		}
		if driver != tasks.WorkerClaudeCode {
			http.Error(w, "rehydrate is only supported for claude-code sessions", http.StatusBadRequest)
			return
		}

		key := tasks.SessionKeyForTask(src)
		ws, ok, err := a.Tasks.GetSessionWorkspace(r.Context(), key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			var fallbacks []string
			if strings.TrimSpace(src.SessionID) != "" {
				fallbacks = append(fallbacks, tasks.SessionKey("", src.SessionID))
			}
			fallbacks = append(fallbacks, tasks.SessionKey(src.ID, ""))
			for _, legacyKey := range fallbacks {
				if strings.TrimSpace(legacyKey) == "" || legacyKey == key {
					continue
				}
				if ws2, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyKey); err == nil && ok2 {
					_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyKey, key)
					if migrated, ok3, err := a.Tasks.GetSessionWorkspace(r.Context(), key); err == nil && ok3 {
						ws = migrated
						ok = true
					} else {
						ws = ws2
						ok = true
					}
					break
				}
			}
		}
		if !ok && strings.TrimSpace(src.SessionID) != "" {
			all, err := a.Tasks.ListTasksWithOptions(r.Context(), 500, tasks.ListTasksOptions{IncludeDeleted: true})
			if err == nil {
				sid := strings.TrimSpace(src.SessionID)
				for _, t := range all {
					if strings.TrimSpace(t.SessionID) != sid {
						continue
					}
					legacyKey := tasks.SessionKey(t.ID, "")
					if strings.TrimSpace(legacyKey) == "" || legacyKey == key {
						continue
					}
					if ws2, ok2, err := a.Tasks.GetSessionWorkspace(r.Context(), legacyKey); err == nil && ok2 {
						_ = a.Tasks.MigrateSessionWorkspaceKey(r.Context(), legacyKey, key)
						if migrated, ok3, err := a.Tasks.GetSessionWorkspace(r.Context(), key); err == nil && ok3 {
							ws = migrated
							ok = true
						} else {
							ws = ws2
							ok = true
						}
						break
					}
				}
			}
		}
		if ok && ws.Status == tasks.WorkspaceStatusActive {
			http.Error(w, "无法新建会话继续：该会话仍有隔离工作区处于 active。请先在 Workspace 面板执行 Merge（把改动合并回 base_workdir）后再重试。", http.StatusConflict)
			return
		}

		prompt := strings.TrimSpace(body.Prompt)
		if prompt == "" {
			prompt = "continue"
		}
		ctxPrompt, err := tasks.BuildRehydratePrompt(r.Context(), a.Tasks, strings.TrimSpace(src.ConversationID), prompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// SafetyEnvelope is an autopilot hint (UI-level “one-time unlock”); it does not count as an explicit safety override.
		explicitSafety := body.UnsafeAutomation ||
			strings.TrimSpace(body.SafetyPreset) != "" ||
			strings.TrimSpace(body.TaskIntent) != "" ||
			strings.TrimSpace(body.CodexSandbox) != "" ||
			strings.TrimSpace(body.CodexApprovalPolicy) != "" ||
			body.CodexSearch ||
			strings.TrimSpace(body.ClaudePermissionMode) != "" ||
			body.ClaudeSandbox ||
			len(body.ClaudeWebFetchDomains) > 0

		unsafe := body.UnsafeAutomation
		safetyEnvelope := strings.TrimSpace(body.SafetyEnvelope)
		safetyPreset := strings.TrimSpace(body.SafetyPreset)
		taskIntent := strings.TrimSpace(body.TaskIntent)
		codexSandbox := strings.TrimSpace(body.CodexSandbox)
		codexApprovalPolicy := strings.TrimSpace(body.CodexApprovalPolicy)
		codexSearch := body.CodexSearch
		claudePermissionMode := strings.TrimSpace(body.ClaudePermissionMode)
		claudeSandbox := body.ClaudeSandbox
		claudeDomains := body.ClaudeWebFetchDomains

		if !explicitSafety {
			unsafe = src.UnsafeAutomation
			safetyPreset = strings.TrimSpace(src.SafetyPreset)
			taskIntent = strings.TrimSpace(src.TaskIntent)
			codexSandbox = strings.TrimSpace(src.CodexSandbox)
			codexApprovalPolicy = strings.TrimSpace(src.CodexApprovalPolicy)
			codexSearch = src.CodexSearch
			claudePermissionMode = strings.TrimSpace(src.ClaudePermissionMode)
			claudeSandbox = src.ClaudeSandbox
			claudeDomains = append([]string{}, src.ClaudeWebFetchDomains...)
		}

		in := tasks.CreateTaskInput{
			WorkerType:            src.WorkerType,
			Mode:                  tasks.ModeNew,
			ConversationID:        src.ConversationID,
			UnsafeAutomation:      unsafe,
			SafetyEnvelope:        safetyEnvelope,
			SafetyPreset:          safetyPreset,
			TaskIntent:            taskIntent,
			CodexSandbox:          codexSandbox,
			CodexApprovalPolicy:   codexApprovalPolicy,
			CodexSearch:           codexSearch,
			ClaudePermissionMode:  claudePermissionMode,
			ClaudeSandbox:         claudeSandbox,
			ClaudeWebFetchDomains: claudeDomains,
			Prompt:                ctxPrompt,
			WorkDir:               src.WorkDir,
			SessionID:             "",
		}

		envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
		llm := runsafe.LLMBackend(nil)
		if a.Observer != nil {
			llm = a.Observer.LLM
		}
		in, ap := runsafe.ApplyAutopilot(r.Context(), in, runsafe.ApplyOptions{
			Driver:   driver,
			Envelope: envelope,
			Classify: runsafe.ClassifyOptions{LLM: llm},
		})

		newTask, err := a.Tasks.CreateTask(r.Context(), in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, fmt.Sprintf("rehydrate: from run=%s session=%s", src.ID, strings.TrimSpace(src.SessionID)))

		if ap.Applied {
			if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
				_, _ = a.Tasks.AppendLog(r.Context(), newTask.ID, tasks.LogSystem, audit)
			}
		}

		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "task.created", Time: time.Now().UTC(), Payload: newTask})
		}
		if a.Workers != nil {
			_ = a.Workers.Start(r.Context(), newTask.ID)
		}
		writeJSON(w, newTask)
	default:
		http.NotFound(w, r)
	}
}

func (a *API) handleChat(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		after := parseInt64(r.URL.Query().Get("after"), 0)
		limit := parseInt(r.URL.Query().Get("limit"), 200)
		msgs, err := a.Chat.List(r.Context(), after, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"messages": msgs})
	case http.MethodPost:
		var body struct {
			Message  string `json:"message"`
			Stream   bool   `json:"stream,omitempty"`
			Backend  string `json:"backend,omitempty"`
			MaxSteps int    `json:"max_steps,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		user := strings.TrimSpace(body.Message)
		if user == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}

		wantStream := body.Stream ||
			strings.TrimSpace(r.URL.Query().Get("stream")) == "1" ||
			strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")

		var (
			flusher http.Flusher
			send    func(event string, payload any)
		)
		if wantStream {
			f, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			flusher = f
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")

			send = func(event string, payload any) {
				data, _ := json.Marshal(payload)
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
				flusher.Flush()
			}

			send("meta", map[string]any{"ok": true})
		}

		userMsg, _ := a.Chat.Append(r.Context(), chat.RoleUser, user)
		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: userMsg})
		}

		if send != nil {
			send("status", map[string]any{"phase": "thinking"})
		}

		var (
			reply observer.Reply
			err   error
		)
		if a.Observer == nil {
			err = errors.New("observer not configured")
		} else {
			reply, err = a.Observer.RespondWithOptions(r.Context(), user, observer.RespondOptions{
				Backend:  body.Backend,
				MaxSteps: body.MaxSteps,
				OnToolCall: func(tool string, args map[string]any) {
					if send != nil {
						send("tool_call", map[string]any{"tool": tool, "args": args})
					}
				},
				OnToolResult: func(tool string, result any) {
					if send != nil {
						send("tool_result", map[string]any{"tool": tool, "result": result})
					}
				},
			})
		}
		if err != nil {
			if send != nil {
				send("error", map[string]any{"error": err.Error()})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		assistantMsg, _ := a.Chat.Append(r.Context(), chat.RoleAssistant, reply.Message)
		if a.Hub != nil {
			a.Hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: assistantMsg})
		}
		if send != nil {
			send("final", map[string]any{"message": reply.Message})
			return
		}
		writeJSON(w, reply)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.Hub == nil {
		http.Error(w, "events not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	eventsCh, unsubscribe := a.Hub.Subscribe(256)
	defer unsubscribe()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	send := func(evt events.Event) {
		data, _ := json.Marshal(evt)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
		flusher.Flush()
	}

	send(events.Event{Type: "hello", Time: time.Now().UTC(), Payload: map[string]any{"ok": true}})

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-eventsCh:
			send(evt)
		case <-heartbeat.C:
			send(events.Event{Type: "heartbeat", Time: time.Now().UTC()})
		}
	}
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

func (a *API) validate() error {
	if a.Tasks == nil || a.Observer == nil || a.Chat == nil {
		return errors.New("api: missing dependencies")
	}
	return nil
}
