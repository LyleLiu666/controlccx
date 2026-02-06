package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/daemon"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/providers"
	"controlccx/internal/tasks"
)

func runSecretaryd(cfg config.Config, secretaryAddr string, runnerBaseURL string) error {
	secretaryAddr = strings.TrimSpace(secretaryAddr)
	if secretaryAddr == "" {
		return errors.New("secretaryd: addr is required")
	}
	runnerBaseURL = strings.TrimRight(strings.TrimSpace(runnerBaseURL), "/")
	if runnerBaseURL == "" {
		return errors.New("secretaryd: runner base url is required")
	}

	lock, err := daemon.AcquireSingleInstanceLock(cfg.Paths.DataDir, "secretaryd", secretaryAddr)
	if err != nil {
		if errors.Is(err, daemon.ErrAlreadyRunning) {
			log.Printf("secretaryd already running; exiting\n")
			return nil
		}
		return err
	}
	defer func() { _ = lock.Release() }()

	ctx := context.Background()
	instanceToken, err := daemon.LoadOrCreateInstanceToken(cfg.Paths.DataDir)
	if err != nil {
		return err
	}
	conn, err := db.Open(ctx, db.Options{Path: cfg.Paths.DBPath})
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	taskStore := tasks.NewStore(conn)
	if err := taskStore.EnsureConversationIDs(ctx); err != nil {
		return err
	}

	authStore, err := auth.Load(filepath.Join(cfg.Paths.DataDir, "secrets.json"))
	if err != nil {
		return err
	}
	chatStore := chat.NewStore(conn)

	providersStore, err := providers.NewStore(cfg.Paths.DataDir)
	if err != nil {
		return err
	}

	runnerClient, err := daemon.NewRunnerClient(runnerBaseURL, daemon.RunnerClientOptions{Token: instanceToken})
	if err != nil {
		return err
	}

	simpleHTTPBackend := observer.NewSimpleHTTPBackendWithProviders(cfg, authStore, providersStore)
	claudeBackend := observer.NewClaudeCLIBackend(cfg, authStore)
	codexBackend := observer.NewCodexCLIBackend(cfg, authStore)

	hub := events.NewHub()
	observerSvc := &observer.Service{
		Store:      taskStore,
		Chat:       chatStore,
		Runner:     runnerClient,
		LLM:        observer.MultiBackend{Backends: []observer.Backend{simpleHTTPBackend, claudeBackend, codexBackend}},
		SimpleHTTP: simpleHTTPBackend,
		Claude:     claudeBackend,
		Codex:      codexBackend,
		ForceAgent: true,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) || !daemon.HasValidInstanceToken(r.Header, instanceToken) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{
			"ok":               true,
			"name":             "secretaryd",
			"protocol_version": daemon.ProtocolVersion,
			"pid":              os.Getpid(),
			"addr":             secretaryAddr,
			"ts_ms":            time.Now().UTC().UnixMilli(),
		})
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) || !daemon.HasValidInstanceToken(r.Header, instanceToken) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		events.ServeSSE(hub)(w, r)
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) || !daemon.HasValidInstanceToken(r.Header, instanceToken) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if chatStore == nil {
			http.Error(w, "chat store not configured", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			after := parseInt64(r.URL.Query().Get("after"), 0)
			limit := parseInt(r.URL.Query().Get("limit"), 200)
			msgs, err := chatStore.List(r.Context(), after, limit)
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

			// Pick up any auth token updates made via the server settings UI.
			if authStore != nil {
				_ = authStore.Reload()
			}
			if providersStore != nil {
				_ = providersStore.Reload()
			}

			body.Backend = resolveSecretaryBackend(body.Backend, providersStore)

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

			userMsg, _ := chatStore.Append(r.Context(), chat.RoleUser, user)
			hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: userMsg})

			if send != nil {
				send("status", map[string]any{"phase": "thinking"})
			}

			var (
				reply observer.Reply
				err   error
			)
			if observerSvc == nil {
				err = errors.New("observer not configured")
			} else {
				reply, err = observerSvc.RespondWithOptions(r.Context(), user, observer.RespondOptions{
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

			assistantMsg, _ := chatStore.Append(r.Context(), chat.RoleAssistant, reply.Message)
			hub.Publish(events.Event{Type: "chat.message", Time: time.Now().UTC(), Payload: assistantMsg})
			if send != nil {
				send("final", map[string]any{"message": reply.Message})
				return
			}
			writeJSON(w, reply)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	srv := &http.Server{
		Addr:         secretaryAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", secretaryAddr)
	if err != nil {
		return err
	}

	go func() {
		log.Printf("controlccx secretaryd listening on http://%s\n", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("secretaryd error: %v", err)
		}
	}()

	waitForShutdown(srv)
	return nil
}
