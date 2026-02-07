package main

import (
	"context"
	"errors"
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
	chatIdem := newChatIdempotencyCache(20*time.Second, 2048)
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

	chatHandler := newSecretaryChatHandler(secretaryChatHandlerDeps{
		Chat:        chatStore,
		Hub:         hub,
		Responder:   observerSvc,
		Auth:        authStore,
		Providers:   providersStore,
		Idempotency: chatIdem,
	})

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
		chatHandler(w, r)
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
