package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"controlccx/internal/daemon"
)

type runnerdHealth struct {
	OK              bool   `json:"ok"`
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
}

func ensureRunnerd(ctx context.Context, runnerBaseURL string, instanceToken string, spawnArgs []string) error {
	runnerBaseURL = strings.TrimRight(strings.TrimSpace(runnerBaseURL), "/")
	if runnerBaseURL == "" {
		return errors.New("runnerd: base url is required")
	}
	instanceToken = strings.TrimSpace(instanceToken)
	if instanceToken == "" {
		return errors.New("runnerd: instance token is required")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	if ok, err := isRunnerdHealthy(checkCtx, runnerBaseURL, instanceToken); err != nil {
		return err
	} else if ok {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("runnerd: executable: %w", err)
	}
	cmd := exec.Command(exe, spawnArgs...)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	daemon.ConfigureDetached(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("runnerd: spawn: %w", err)
	}

	// Wait until healthy (best-effort).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		ok, err := isRunnerdHealthy(checkCtx, runnerBaseURL, instanceToken)
		cancel()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("runnerd: did not become healthy at %s", runnerBaseURL)
}

func isRunnerdHealthy(ctx context.Context, runnerBaseURL string, instanceToken string) (bool, error) {
	u := strings.TrimRight(runnerBaseURL, "/") + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(daemon.InstanceTokenHeader, strings.TrimSpace(instanceToken))
	client := &http.Client{Timeout: 350 * time.Millisecond}
	res, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		return false, fmt.Errorf("runnerd: instance token mismatch at %s (hint: stop existing runnerd or use the same data dir)", runnerBaseURL)
	}
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("runnerd: unexpected /health status %d at %s", res.StatusCode, runnerBaseURL)
	}
	var out runnerdHealth
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, fmt.Errorf("runnerd: decode /health response: %w", err)
	}
	if !out.OK || strings.TrimSpace(out.Name) != "runnerd" {
		return false, fmt.Errorf("runnerd: unexpected /health payload at %s", runnerBaseURL)
	}
	if out.ProtocolVersion != daemon.ProtocolVersion {
		return false, fmt.Errorf("runnerd: protocol mismatch at %s (got=%d want=%d)", runnerBaseURL, out.ProtocolVersion, daemon.ProtocolVersion)
	}
	return true, nil
}
