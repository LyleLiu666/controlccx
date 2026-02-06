package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func shouldAutoOpenBrowser(noOpenFlag bool) bool {
	if noOpenFlag {
		return false
	}
	v := strings.TrimSpace(os.Getenv("CONTROLCCX_NO_OPEN"))
	if v == "" {
		return true
	}
	v = strings.ToLower(v)
	return !(v == "1" || v == "true" || v == "yes" || v == "y" || v == "on")
}

func openURLForListenAddr(listenAddr string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return "", fmt.Errorf("parse listen addr %q: %w", listenAddr, err)
	}
	host = strings.TrimSpace(host)
	openHost := host
	// Map non-routable bind hosts to a reachable localhost.
	// Users cannot open 0.0.0.0 / :: in a browser.
	if openHost == "" || openHost == "0.0.0.0" || openHost == "::" {
		openHost = "127.0.0.1"
	}
	u := (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(openHost, port),
	}).String()
	return u, nil
}

func isControlCCXRunning(ctx context.Context, baseURL string) bool {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return false
	}
	u := baseURL + "/api/auth/status"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 600 * time.Millisecond}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return false
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return false
	}
	_, ok1 := payload["claude"].(map[string]any)
	_, ok2 := payload["codex"].(map[string]any)
	return ok1 && ok2
}

func browserOpenCommandForGOOS(goos, targetURL string) (string, []string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", nil, errors.New("url is empty")
	}
	switch goos {
	case "darwin":
		return "open", []string{targetURL}, nil
	case "windows":
		// Avoid cmd.exe quoting pitfalls; rundll32 is stable for URLs.
		return "rundll32", []string{"url.dll,FileProtocolHandler", targetURL}, nil
	default:
		// Linux / BSD etc.
		return "xdg-open", []string{targetURL}, nil
	}
}

func openBrowserBestEffort(targetURL string) error {
	name, args, err := browserOpenCommandForGOOS(runtime.GOOS, targetURL)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	// Detach: do not inherit stdio and do not block startup/shutdown.
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
