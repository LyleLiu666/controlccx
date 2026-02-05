package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"controlccx"
	"controlccx/internal/api"
	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/skills"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
	"controlccx/internal/worker"
)

func main() {
	dataDirFlag := flag.String("data-dir", "", "data directory (default: ~/.controlccx)")
	addrFlag := flag.String("addr", "", "listen address (default from config.yaml)")
	staticDirFlag := flag.String("static-dir", "", "directory for built web assets (override embedded assets)")
	claudePathFlag := flag.String("claude-path", "", "path to claude executable (optional)")
	codexPathFlag := flag.String("codex-path", "", "path to codex executable (optional)")
	gitBashPathFlag := flag.String("gitbash-path", "", "path to Git Bash bash.exe on Windows (optional)")
	flag.Parse()

	cfg, err := config.Load(*dataDirFlag)
	if err != nil {
		log.Fatal(err)
	}

	if *addrFlag != "" {
		cfg.Server.Addr = *addrFlag
	}
	if *claudePathFlag != "" {
		cfg.Paths.Claude = *claudePathFlag
	}
	if *codexPathFlag != "" {
		cfg.Paths.Codex = *codexPathFlag
	}
	if *gitBashPathFlag != "" {
		cfg.Paths.GitBash = *gitBashPathFlag
	}

	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: cfg.Paths.DBPath})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	taskStore := tasks.NewStore(conn)
	if _, err := taskStore.MarkInterrupted(ctx); err != nil {
		log.Fatal(err)
	}
	if err := taskStore.EnsureConversationIDs(ctx); err != nil {
		log.Fatal(err)
	}

	authStore, err := auth.Load(filepath.Join(cfg.Paths.DataDir, "secrets.json"))
	if err != nil {
		log.Fatal(err)
	}

	toolsSvc, err := tooling.NewService(tooling.Options{
		DataDir:  cfg.Paths.DataDir,
		Defaults: tooling.DefaultsFromConfig(cfg),
	})
	if err != nil {
		log.Fatal(err)
	}

	hub := events.NewHub()
	workerMgr := worker.NewManager(cfg, taskStore, hub, authStore, toolsSvc)
	chatStore := chat.NewStore(conn)
	claudeBackend := observer.NewClaudeCLIBackend(cfg, authStore)
	codexBackend := observer.NewCodexCLIBackend(cfg, authStore)
	observerSvc := &observer.Service{
		Store:      taskStore,
		Chat:       chatStore,
		Runner:     workerMgr,
		LLM:        observer.MultiBackend{Backends: []observer.Backend{claudeBackend, codexBackend}},
		Claude:     claudeBackend,
		Codex:      codexBackend,
		ForceAgent: true,
	}

	skillsSvc, err := skills.NewService(skills.Options{})
	if err != nil {
		log.Fatal(err)
	}
	skillVersionsSvc, err := skills.NewVersionsService(skills.VersionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	perSkillVersionsSvc, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	autoScan := skills.NewAutoVersionScanner(skillsSvc, perSkillVersionsSvc, skills.AutoVersionScanOptions{})

	apiSvc := &api.API{
		Tasks:                taskStore,
		Workers:              workerMgr,
		Observer:             observerSvc,
		Chat:                 chatStore,
		Hub:                  hub,
		Auth:                 authStore,
		Skills:               skillsSvc,
		SkillVersions:        skillVersionsSvc,
		SkillVersionsBySkill: perSkillVersionsSvc,
		SkillAutoVersionScan: autoScan,
		Tools:                toolsSvc,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiSvc.Handler())
	mux.Handle("/api", apiSvc.Handler())
	mux.Handle("/", spaOrFallback(resolveUIFS(*staticDirFlag)))

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	if autoScan != nil {
		stopAutoScan := make(chan struct{})
		srv.RegisterOnShutdown(func() {
			close(stopAutoScan)
		})

		// Kick off a best-effort scan on startup (async).
		autoScan.TriggerAsync(context.Background(), true)

		// Scheduled scan every 3 hours.
		go func() {
			ticker := time.NewTicker(3 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					autoScan.TriggerAsync(context.Background(), true)
				case <-stopAutoScan:
					return
				}
			}
		}()
	}

	go func() {
		log.Printf("controlccx server listening on http://%s\n", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	waitForShutdown(srv)
}

func waitForShutdown(srv *http.Server) {
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func resolveUIFS(staticDirFlag string) fs.FS {
	staticDir := strings.TrimSpace(staticDirFlag)
	if staticDir != "" {
		return os.DirFS(filepath.Clean(staticDir))
	}
	return controlccx.WebDistFS()
}

func spaOrFallback(fsys fs.FS) http.Handler {
	if fsys == nil {
		return missingUIHandler()
	}

	indexExists := fsExists(fsys, "index.html")
	fileServer := http.FileServer(http.FS(fsys))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "controlccx: failed to read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(b)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if fsExists(fsys, path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		if !indexExists {
			missingUIHandler().ServeHTTP(w, r)
			return
		}
		// SPA fallback: return index.html while keeping the original URL (so client-side routing works).
		//
		// NOTE: we cannot delegate to http.FileServer with URL.Path="/index.html" because it will
		// issue a 301 redirect to "./" for index.html requests, which breaks deep links like "/skills".
		serveIndex(w, r)
	})
}

func newCopyURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	v := *u
	return &v
}

func fsExists(fsys fs.FS, name string) bool {
	_, err := fs.Stat(fsys, name)
	return err == nil
}

func missingUIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintln(w, "controlccx: web UI not built yet; run `pnpm build` or `pnpm dev`.")
			return
		}
		http.NotFound(w, r)
	})
}
