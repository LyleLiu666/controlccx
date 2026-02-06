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

type secretarydHealth struct {
	OK              bool   `json:"ok"`
	Name            string `json:"name"`
	ProtocolVersion int    `json:"protocol_version"`
}

func ensureSecretaryd(ctx context.Context, secretaryBaseURL string, instanceToken string, spawnArgs []string) error {
	secretaryBaseURL = strings.TrimRight(strings.TrimSpace(secretaryBaseURL), "/")
	if secretaryBaseURL == "" {
		return errors.New("secretaryd: base url is required")
	}
	instanceToken = strings.TrimSpace(instanceToken)
	if instanceToken == "" {
		return errors.New("secretaryd: instance token is required")
	}

	checkCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	if ok, err := isSecretarydHealthy(checkCtx, secretaryBaseURL, instanceToken); err != nil {
		return err
	} else if ok {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("secretaryd: executable: %w", err)
	}
	cmd := exec.Command(exe, spawnArgs...)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	daemon.ConfigureDetached(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("secretaryd: spawn: %w", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		checkCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		ok, err := isSecretarydHealthy(checkCtx, secretaryBaseURL, instanceToken)
		cancel()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("secretaryd: did not become healthy at %s", secretaryBaseURL)
}

func isSecretarydHealthy(ctx context.Context, secretaryBaseURL string, instanceToken string) (bool, error) {
	u := strings.TrimRight(secretaryBaseURL, "/") + "/health"
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
		return false, fmt.Errorf("secretaryd: instance token mismatch at %s (hint: stop existing secretaryd or use the same data dir)", secretaryBaseURL)
	}
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("secretaryd: unexpected /health status %d at %s", res.StatusCode, secretaryBaseURL)
	}
	var out secretarydHealth
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, fmt.Errorf("secretaryd: decode /health response: %w", err)
	}
	if !out.OK || strings.TrimSpace(out.Name) != "secretaryd" {
		return false, fmt.Errorf("secretaryd: unexpected /health payload at %s", secretaryBaseURL)
	}
	if out.ProtocolVersion != daemon.ProtocolVersion {
		return false, fmt.Errorf("secretaryd: protocol mismatch at %s (got=%d want=%d)", secretaryBaseURL, out.ProtocolVersion, daemon.ProtocolVersion)
	}
	return true, nil
}
