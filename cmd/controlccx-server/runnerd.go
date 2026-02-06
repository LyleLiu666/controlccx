package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/daemon"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
	"controlccx/internal/worker"
)

func runRunnerd(cfg config.Config, runnerAddr string) error {
	runnerAddr = strings.TrimSpace(runnerAddr)
	if runnerAddr == "" {
		return errors.New("runnerd: addr is required")
	}

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
	if _, err := taskStore.MarkInterrupted(ctx); err != nil {
		return err
	}
	if err := taskStore.EnsureConversationIDs(ctx); err != nil {
		return err
	}

	authStore, err := auth.Load(filepath.Join(cfg.Paths.DataDir, "secrets.json"))
	if err != nil {
		return err
	}

	toolsSvc, err := tooling.NewService(tooling.Options{
		DataDir:  cfg.Paths.DataDir,
		Defaults: tooling.DefaultsFromConfig(cfg),
	})
	if err != nil {
		return err
	}

	hub := events.NewHub()
	workerMgr := worker.NewManager(cfg, taskStore, hub, authStore, toolsSvc)

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
			"name":             "runnerd",
			"protocol_version": daemon.ProtocolVersion,
			"pid":              os.Getpid(),
			"addr":             runnerAddr,
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
	mux.HandleFunc("/api/runner/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) || !daemon.HasValidInstanceToken(r.Header, instanceToken) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/runner/tasks/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		taskID := strings.TrimSpace(parts[0])
		action := strings.TrimSpace(parts[1])
		if taskID == "" || action == "" {
			http.NotFound(w, r)
			return
		}

		switch action {
		case "start":
			if authStore != nil {
				_ = authStore.Reload()
			}
			if toolsSvc != nil {
				_ = toolsSvc.Reload()
			}
			if err := workerMgr.Start(r.Context(), taskID); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
		case "cancel":
			ok, _ := workerMgr.Cancel(r.Context(), taskID)
			writeJSON(w, map[string]any{"ok": ok})
		default:
			http.NotFound(w, r)
		}
	})

	srv := &http.Server{
		Addr:         runnerAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	ln, err := net.Listen("tcp", runnerAddr)
	if err != nil {
		return err
	}

	go func() {
		log.Printf("controlccx runnerd listening on http://%s\n", ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("runnerd error: %v", err)
		}
	}()

	waitForShutdown(srv)
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func isLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
