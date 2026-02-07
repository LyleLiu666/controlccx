package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/providers"
)

type secretaryChatResponder interface {
	RespondWithOptions(ctx context.Context, userMessage string, opts observer.RespondOptions) (observer.Reply, error)
}

type secretaryChatHandlerDeps struct {
	Chat        *chat.Store
	Hub         *events.Hub
	Responder   secretaryChatResponder
	Auth        *auth.Store
	Providers   *providers.Store
	Idempotency *chatIdempotencyCache
	HistoryLog  *secretaryChatHistoryLogger
}

func newSecretaryChatHandler(deps secretaryChatHandlerDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Chat == nil {
			http.Error(w, "chat store not configured", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			after := parseInt64(r.URL.Query().Get("after"), 0)
			limit := parseInt(r.URL.Query().Get("limit"), 200)
			msgs, err := deps.Chat.List(r.Context(), after, limit)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"messages": msgs})
		case http.MethodPost:
			var body struct {
				Message         string `json:"message"`
				Stream          bool   `json:"stream,omitempty"`
				Backend         string `json:"backend,omitempty"`
				MaxSteps        int    `json:"max_steps,omitempty"`
				IdempotencyKey  string `json:"idempotency_key,omitempty"`
				IdempotencyKey2 string `json:"idempotencyKey,omitempty"` // back-compat (clients shouldn't use)
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

			// Pick up any auth token updates made via the server settings UI.
			if deps.Auth != nil {
				_ = deps.Auth.Reload()
			}
			if deps.Providers != nil {
				_ = deps.Providers.Reload()
			}

			body.Backend = resolveSecretaryBackend(body.Backend, deps.Providers)

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

			idKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if idKey == "" {
				idKey = strings.TrimSpace(body.IdempotencyKey)
			}
			if idKey == "" {
				idKey = strings.TrimSpace(body.IdempotencyKey2)
			}
			if len(idKey) > 200 {
				idKey = idKey[:200]
			}

			lease, cached, idemErr := deps.Idempotency.Acquire(r.Context(), idKey)
			if idemErr != nil {
				respondSecretaryChatError(w, send, idemErr)
				return
			}
			if cached != nil {
				if deps.HistoryLog != nil {
					msgs, _ := listAllChatMessages(r.Context(), deps.Chat)
					_ = deps.HistoryLog.Append(secretaryChatHistoryLogEntry{
						Time:           time.Now().UTC(),
						Kind:           "idempotency_cache_hit",
						IdempotencyKey: idKey,
						Backend:        body.Backend,
						MaxSteps:       body.MaxSteps,
						Stream:         send != nil,
						UserMessage:    user,
						Cached:         true,
						Reply:          cached.Reply,
						Error:          cached.Error,
						Messages:       msgs,
					})
				}
				respondSecretaryChatCached(w, send, cached)
				return
			}

			userMsg, _ := deps.Chat.Append(r.Context(), chat.RoleUser, user)
			if deps.Hub != nil {
				deps.Hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: userMsg})
			}

			if deps.HistoryLog != nil {
				msgs, _ := listAllChatMessages(r.Context(), deps.Chat)
				_ = deps.HistoryLog.Append(secretaryChatHistoryLogEntry{
					Time:           time.Now().UTC(),
					Kind:           "llm_request",
					IdempotencyKey: idKey,
					Backend:        body.Backend,
					MaxSteps:       body.MaxSteps,
					Stream:         send != nil,
					UserMessage:    user,
					Messages:       msgs,
				})
			}

			if send != nil {
				send("status", map[string]any{"phase": "thinking"})
			}

			if deps.Responder == nil {
				err := errors.New("observer not configured")
				lease.Finish("", err)
				respondSecretaryChatError(w, send, err)
				return
			}
			reply, err := deps.Responder.RespondWithOptions(r.Context(), user, observer.RespondOptions{
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
			if err != nil {
				lease.Finish("", err)
				respondSecretaryChatError(w, send, err)
				return
			}

			assistantMsg, _ := deps.Chat.Append(r.Context(), chat.RoleAssistant, reply.Message)
			if deps.Hub != nil {
				deps.Hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: assistantMsg})
			}
			lease.Finish(reply.Message, nil)

			if send != nil {
				send("final", map[string]any{"message": reply.Message})
				return
			}
			writeJSON(w, reply)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func respondSecretaryChatCached(w http.ResponseWriter, send func(event string, payload any), cached *chatIdempotencyResult) {
	if cached == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if cached.Error != "" {
		respondSecretaryChatError(w, send, errors.New(cached.Error))
		return
	}
	if send != nil {
		send("final", map[string]any{"message": cached.Reply})
		return
	}
	writeJSON(w, observer.Reply{Message: cached.Reply})
}

func respondSecretaryChatError(w http.ResponseWriter, send func(event string, payload any), err error) {
	if send != nil {
		send("error", map[string]any{"error": err.Error()})
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
