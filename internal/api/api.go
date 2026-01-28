package api

import (
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
	"controlccx/internal/systeminfo"
	"controlccx/internal/tasks"
	"controlccx/internal/worker"
)

type API struct {
	Tasks    *tasks.Store
	Workers  *worker.Manager
	Observer *observer.Service
	Chat     *chat.Store
	Hub      *events.Hub
	FSRoots  []FSRoot
	Auth     *auth.Store
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system", a.handleSystem)
	mux.HandleFunc("/api/tasks", a.handleTasks)
	mux.HandleFunc("/api/tasks/", a.handleTaskByID)
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
		items, err := a.Tasks.ListTasks(r.Context(), limit)
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
		task, err := a.Tasks.CreateTask(r.Context(), in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
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
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		after := parseInt64(r.URL.Query().Get("after"), 0)
		limit := parseInt(r.URL.Query().Get("limit"), 500)
		logs, err := a.Tasks.ListLogs(r.Context(), id, after, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"logs": logs})
	case "resume":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Prompt           string `json:"prompt"`
			UnsafeAutomation bool   `json:"unsafe_automation,omitempty"`
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
		if strings.TrimSpace(prev.SessionID) == "" {
			http.Error(w, "task has no session_id to resume", http.StatusBadRequest)
			return
		}
		newTask, err := a.Tasks.CreateTask(r.Context(), tasks.CreateTaskInput{
			WorkerType: prev.WorkerType,
			Mode:       tasks.ModeResume,
			UnsafeAutomation: body.UnsafeAutomation,
			Prompt:     body.Prompt,
			WorkDir:    prev.WorkDir,
			SessionID:  prev.SessionID,
			Warning:    prev.Warning,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
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

func (a *API) validate() error {
	if a.Tasks == nil || a.Observer == nil || a.Chat == nil {
		return errors.New("api: missing dependencies")
	}
	return nil
}
