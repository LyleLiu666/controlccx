package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"controlccx/internal/api"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/events"
	"controlccx/internal/observer"
	"controlccx/internal/tasks"
	"controlccx/internal/worker"
)

func main() {
	dataDirFlag := flag.String("data-dir", "", "data directory (default: ~/.controlccx)")
	addrFlag := flag.String("addr", "", "listen address (default from config.yaml)")
	staticDirFlag := flag.String("static-dir", "", "directory for built web assets (default: web/dist if present)")
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

	staticDir := strings.TrimSpace(*staticDirFlag)
	if staticDir == "" {
		staticDir = defaultStaticDir()
	}
	staticDir = filepath.Clean(staticDir)

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

	hub := events.NewHub()
	workerMgr := worker.NewManager(cfg, taskStore, hub)
	observerSvc := &observer.Service{Store: taskStore}
	chatStore := chat.NewStore(conn)

	apiSvc := &api.API{
		Tasks:    taskStore,
		Workers:  workerMgr,
		Observer: observerSvc,
		Chat:     chatStore,
		Hub:      hub,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", apiSvc.Handler())
	mux.Handle("/api", apiSvc.Handler())
	mux.Handle("/", spaOrFallback(staticDir))

	srv := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
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

func defaultStaticDir() string {
	if stat, err := os.Stat("web/dist"); err == nil && stat.IsDir() {
		return "web/dist"
	}
	return ""
}

func spaOrFallback(staticDir string) http.Handler {
	if staticDir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = fmt.Fprintln(w, "controlccx: web assets not found; run pnpm -C web dev or build web/dist")
				return
			}
			http.NotFound(w, r)
		})
	}

	root := http.Dir(staticDir)
	fs := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := root.Open(path)
		if err == nil {
			_ = f.Close()
		}
		if err != nil {
			// SPA fallback.
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = newCopyURL(r.URL)
			r2.URL.Path = "/index.html"
			fs.ServeHTTP(w, r2)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func newCopyURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	v := *u
	return &v
}
