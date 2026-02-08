package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"controlccx/internal/agentsdk"
	"controlccx/internal/secretary"
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

func (a *API) handleSecretaryMessagesStream(w http.ResponseWriter, r *http.Request) {
	if a.Secretary == nil {
		http.Error(w, "secretary not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	emit := func(event string, payload any) {
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	emit("start", map[string]any{"ok": true})

	hooks := &secretary.SendHooks{
		OnVisibleDelta: func(delta string) error {
			if delta == "" {
				return nil
			}
			emit("delta", map[string]any{"delta": delta})
			return nil
		},
		OnTrace: func(step int, message string) {
			line := strings.TrimSpace(message)
			if line == "" {
				return
			}
			emit("thinking", map[string]any{
				"kind": "trace",
				"step": step,
				"line": line,
			})
		},
		OnToolCall: func(step int, event agentsdk.ToolCallEvent) {
			name := strings.TrimSpace(event.Name)
			if name == "" {
				name = "unknown"
			}
			emit("thinking", map[string]any{
				"kind":      "tool_call",
				"step":      step,
				"tool_name": name,
				"line":      fmt.Sprintf("调用工具：%s", name),
			})
		},
		OnToolResult: func(step int, event agentsdk.ToolResultEvent) {
			name := strings.TrimSpace(event.ToolName)
			if name == "" {
				name = "unknown"
			}
			line := fmt.Sprintf("工具完成：%s", name)
			if !event.OK {
				errText := strings.TrimSpace(event.Error)
				if errText != "" {
					line = fmt.Sprintf("工具失败：%s（%s）", name, errText)
				} else {
					line = fmt.Sprintf("工具失败：%s", name)
				}
			}
			emit("thinking", map[string]any{
				"kind":      "tool_result",
				"step":      step,
				"tool_name": name,
				"ok":        event.OK,
				"line":      line,
				"error":     strings.TrimSpace(event.Error),
			})
		},
		OnError: func(step int, message string) {
			line := strings.TrimSpace(message)
			if line == "" {
				return
			}
			emit("thinking", map[string]any{
				"kind":  "error",
				"step":  step,
				"line":  line,
				"error": line,
			})
		},
	}

	reply, err := a.Secretary.SendStream(r.Context(), msg, hooks)
	if err != nil {
		emit("error", map[string]any{"error": err.Error()})
		return
	}
	emit("done", map[string]any{"reply": reply})
}
