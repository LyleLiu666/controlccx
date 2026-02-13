package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
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
	"controlccx/internal/audit"
	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/daemon"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/providers"
	"controlccx/internal/runworkspace"
	"controlccx/internal/secretary"
	"controlccx/internal/secretary/llm"
	"controlccx/internal/skills"
	"controlccx/internal/taskops"
	"controlccx/internal/tasks"
	"controlccx/internal/tooling"
)

func main() {
	modeFlag := flag.String("mode", "server", "run mode: server | runnerd")
	dataDirFlag := flag.String("data-dir", "", "data directory (default: ~/.controlccx)")
	addrFlag := flag.String("addr", "", "listen address (default from config.yaml)")
	runnerAddrFlag := flag.String("runnerd-addr", "127.0.0.1:5175", "runner daemon listen address (server attaches/spawns; runnerd listens here)")
	staticDirFlag := flag.String("static-dir", "", "directory for built web assets (override embedded assets)")
	claudePathFlag := flag.String("claude-path", "", "path to claude executable (optional)")
	codexPathFlag := flag.String("codex-path", "", "path to codex executable (optional)")
	gitBashPathFlag := flag.String("gitbash-path", "", "path to Git Bash bash.exe on Windows (optional)")
	noOpenFlag := flag.Bool("no-open", false, "do not auto-open the web UI in a browser on startup (or set CONTROLCCX_NO_OPEN=1)")
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

	mode := strings.ToLower(strings.TrimSpace(*modeFlag))
	switch mode {
	case "", "server":
		// continue below
	case "runnerd":
		if err := runRunnerd(cfg, *runnerAddrFlag); err != nil {
			log.Fatal(err)
		}
		return
	default:
		log.Fatalf("unknown mode: %q", mode)
	}

	autoOpen := shouldAutoOpenBrowser(*noOpenFlag)
	initialOpenURL, openURLErr := openURLForListenAddr(cfg.Server.Addr)
	if openURLErr == nil {
		checkCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
		defer cancel()
		if isControlCCXRunning(checkCtx, initialOpenURL) {
			log.Printf("controlccx already running at %s\n", initialOpenURL)
			if autoOpen {
				_ = openBrowserBestEffort(initialOpenURL)
			}
			return
		}
	}

	instanceToken, err := daemon.LoadOrCreateInstanceToken(cfg.Paths.DataDir)
	if err != nil {
		log.Fatal(err)
	}

	runnerBaseURL, err := openURLForListenAddr(*runnerAddrFlag)
	if err != nil {
		log.Fatal(err)
	}
	spawnArgs := []string{
		"--mode", "runnerd",
		"--data-dir", cfg.Paths.DataDir,
		"--runnerd-addr", strings.TrimSpace(*runnerAddrFlag),
	}
	if strings.TrimSpace(*claudePathFlag) != "" {
		spawnArgs = append(spawnArgs, "--claude-path", strings.TrimSpace(*claudePathFlag))
	}
	if strings.TrimSpace(*codexPathFlag) != "" {
		spawnArgs = append(spawnArgs, "--codex-path", strings.TrimSpace(*codexPathFlag))
	}
	if strings.TrimSpace(*gitBashPathFlag) != "" {
		spawnArgs = append(spawnArgs, "--gitbash-path", strings.TrimSpace(*gitBashPathFlag))
	}
	if err := ensureRunnerd(ctx, runnerBaseURL, instanceToken, spawnArgs); err != nil {
		log.Fatal(err)
	}
	runnerClient, err := daemon.NewRunnerClient(runnerBaseURL, daemon.RunnerClientOptions{Token: instanceToken})
	if err != nil {
		log.Fatal(err)
	}

	conn, err := db.Open(ctx, db.Options{Path: cfg.Paths.DBPath})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	taskStore := tasks.NewStore(conn)
	if err := taskStore.EnsureConversationIDs(ctx); err != nil {
		log.Fatal(err)
	}
	auditSvc := audit.NewService(conn, audit.Options{
		Retention: audit.RetentionOptions{
			Days:         cfg.AuditRetentionDays,
			MaxRows:      cfg.AuditMaxRowsPerSource,
			GCInterval:   parseAuditGCInterval(cfg.AuditGCInterval),
			StartupRunGC: true,
		},
	})

	chatStore := chat.NewStore(conn)
	secretaryEvents := secretary.NewEventStore(conn)
	secretaryCompressions := secretary.NewCompressionStore(conn)

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

	providersStore, err := providers.NewStore(cfg.Paths.DataDir)
	if err != nil {
		log.Fatal(err)
	}

	hub := events.NewHub()
	autopilotLLM := llm.NewProviderBackendWithProviders(cfg, authStore, providersStore)
	opsSvc := &taskops.Service{
		Tasks:            taskStore,
		Workers:          runnerClient,
		Hub:              hub,
		Tools:            toolsSvc,
		AutopilotLLM:     autopilotLLM,
		AutopilotTimeout: 2 * time.Second,
	}
	fsRoots := api.FSRootsFromPaths(cfg.FSRoots)

	skillsSvc, err := skills.NewService(skills.Options{})
	if err != nil {
		log.Fatal(err)
	}

	secretarySvc := secretary.NewService(
		cfg,
		taskStore,
		chatStore,
		authStore,
		providersStore,
		secretary.WithEventStore(secretaryEvents),
		secretary.WithCompressionStore(secretaryCompressions),
		secretary.WithEventHub(hub),
		secretary.WithTaskOps(opsSvc),
		secretary.WithSkills(skillsSvc),
		secretary.WithFSRoots(cfg.FSRoots),
	)

	skillVersionsSvc, err := skills.NewVersionsService(skills.VersionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	perSkillVersionsSvc, err := skills.NewPerSkillVersionsService(skills.PerSkillVersionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	autoScan := skills.NewAutoVersionScanner(skillsSvc, perSkillVersionsSvc, skills.AutoVersionScanOptions{})
	workspacesSvc := runworkspace.NewService(taskStore, runworkspace.Options{})

	apiSvc := &api.API{
		Tasks:                taskStore,
		Workers:              runnerClient,
		Hub:                  hub,
		FSRoots:              fsRoots,
		Audit:                auditSvc,
		Auth:                 authStore,
		InstanceToken:        instanceToken,
		Providers:            providersStore,
		Secretary:            secretarySvc,
		Skills:               skillsSvc,
		SkillVersions:        skillVersionsSvc,
		SkillVersionsBySkill: perSkillVersionsSvc,
		SkillAutoVersionScan: autoScan,
		Tools:                toolsSvc,
		Workspaces:           workspacesSvc,
		TaskOps:              opsSvc,
	}
	stopBackgroundLoops := apiSvc.StartBackgroundLoops(context.Background())

	mux := http.NewServeMux()
	mux.Handle("/api/control-plane", withInstanceTokenGate(cfg.Server.Addr, instanceToken, http.HandlerFunc(controlPlaneHandler(runnerBaseURL, instanceToken))))
	apiHandler := withInstanceTokenGate(cfg.Server.Addr, instanceToken, apiSvc.Handler())
	mux.Handle("/api/", apiHandler)
	mux.Handle("/api", apiHandler)
	mux.Handle("/", spaOrFallback(resolveUIFS(*staticDirFlag)))

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	bridgeCtx, cancelBridge := context.WithCancel(context.Background())
	srv.RegisterOnShutdown(cancelBridge)
	srv.RegisterOnShutdown(audit.StartGCLoop(auditSvc, log.Printf))
	srv.RegisterOnShutdown(stopBackgroundLoops)
	srv.RegisterOnShutdown(secretarySvc.StartTaskStatusReporter(context.Background(), hub))
	go func() {
		if err := daemon.BridgeSSEToHub(bridgeCtx, runnerBaseURL+"/api/events", hub, daemon.SSEBridgeOptions{
			Logf:  log.Printf,
			Token: instanceToken,
		}); err != nil {
			log.Printf("runner SSE bridge stopped: %v", err)
		}
	}()

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

	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		// Port in use: check again for a running ControlCCX instance before failing.
		if openURLErr == nil {
			checkCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
			defer cancel()
			if isControlCCXRunning(checkCtx, initialOpenURL) {
				log.Printf("controlccx already running at %s\n", initialOpenURL)
				if autoOpen {
					_ = openBrowserBestEffort(initialOpenURL)
				}
				return
			}
		}
		log.Fatal(err)
	}

	finalListenAddr := ln.Addr().String()
	openURL, err := openURLForListenAddr(finalListenAddr)
	if err != nil {
		openURL = "http://" + finalListenAddr
	}

	go func() {
		log.Printf("controlccx server listening on %s\n", openURL)
		if autoOpen {
			_ = openBrowserBestEffort(openURL)
		}
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
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

func parseAuditGCInterval(raw string) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Hour
	}
	dur, err := time.ParseDuration(value)
	if err != nil || dur <= 0 {
		return time.Hour
	}
	return dur
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
