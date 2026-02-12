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
	BinaryStamp     string `json:"binary_stamp,omitempty"`
	PID             int    `json:"pid,omitempty"`
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

	expectedBinaryStamp := currentExecutableBinaryStampBestEffort()

	checkCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	health, ok, err := isRunnerdHealthy(checkCtx, runnerBaseURL, instanceToken)
	if err != nil {
		return err
	}
	if ok && !needsRunnerdRestart(expectedBinaryStamp, health.BinaryStamp) {
		return nil
	}

	if ok && needsRunnerdRestart(expectedBinaryStamp, health.BinaryStamp) {
		if health.PID <= 0 {
			return fmt.Errorf("runnerd: binary mismatch at %s but daemon pid is unavailable (got=%q want=%q)", runnerBaseURL, strings.TrimSpace(health.BinaryStamp), strings.TrimSpace(expectedBinaryStamp))
		}
		if err := killProcessByPID(health.PID); err != nil {
			return fmt.Errorf("runnerd: restart stale daemon pid=%d: %w", health.PID, err)
		}
		if err := waitRunnerdDown(ctx, runnerBaseURL, instanceToken, 2*time.Second); err != nil {
			return err
		}
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
		health, ok, err := isRunnerdHealthy(checkCtx, runnerBaseURL, instanceToken)
		cancel()
		if err != nil {
			return err
		}
		if ok {
			if needsRunnerdRestart(expectedBinaryStamp, health.BinaryStamp) {
				return fmt.Errorf("runnerd: healthy but binary mismatch at %s (got=%q want=%q)", runnerBaseURL, strings.TrimSpace(health.BinaryStamp), strings.TrimSpace(expectedBinaryStamp))
			}
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("runnerd: did not become healthy at %s", runnerBaseURL)
}

func needsRunnerdRestart(expectedBinaryStamp string, observedBinaryStamp string) bool {
	expectedBinaryStamp = strings.TrimSpace(expectedBinaryStamp)
	if expectedBinaryStamp == "" {
		return false
	}
	return strings.TrimSpace(observedBinaryStamp) != expectedBinaryStamp
}

func killProcessByPID(pid int) error {
	if pid <= 0 {
		return errors.New("pid must be > 0")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func waitRunnerdDown(ctx context.Context, runnerBaseURL string, instanceToken string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 1 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		_, ok, err := isRunnerdHealthy(checkCtx, runnerBaseURL, instanceToken)
		cancel()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		time.Sleep(90 * time.Millisecond)
	}
	return fmt.Errorf("runnerd: daemon did not stop at %s", runnerBaseURL)
}

func isRunnerdHealthy(ctx context.Context, runnerBaseURL string, instanceToken string) (runnerdHealth, bool, error) {
	u := strings.TrimRight(runnerBaseURL, "/") + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return runnerdHealth{}, false, nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(daemon.InstanceTokenHeader, strings.TrimSpace(instanceToken))
	client := &http.Client{Timeout: 350 * time.Millisecond}
	res, err := client.Do(req)
	if err != nil {
		return runnerdHealth{}, false, nil
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		return runnerdHealth{}, false, fmt.Errorf("runnerd: instance token mismatch at %s (hint: stop existing runnerd or use the same data dir)", runnerBaseURL)
	}
	if res.StatusCode != http.StatusOK {
		return runnerdHealth{}, false, fmt.Errorf("runnerd: unexpected /health status %d at %s", res.StatusCode, runnerBaseURL)
	}
	var out runnerdHealth
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return runnerdHealth{}, false, fmt.Errorf("runnerd: decode /health response: %w", err)
	}
	if !out.OK || strings.TrimSpace(out.Name) != "runnerd" {
		return runnerdHealth{}, false, fmt.Errorf("runnerd: unexpected /health payload at %s", runnerBaseURL)
	}
	if out.ProtocolVersion != daemon.ProtocolVersion {
		return runnerdHealth{}, false, fmt.Errorf("runnerd: protocol mismatch at %s (got=%d want=%d)", runnerBaseURL, out.ProtocolVersion, daemon.ProtocolVersion)
	}
	return out, true, nil
}
