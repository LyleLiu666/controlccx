package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"controlccx/internal/config"
	"controlccx/internal/daemon"
)

func TestStartupIntegration_FirstAndSecondLaunch_AttachOrSpawn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dataDir := t.TempDir()
	cfg, err := config.Load(dataDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	instanceToken, err := daemon.LoadOrCreateInstanceToken(cfg.Paths.DataDir)
	if err != nil {
		t.Fatalf("LoadOrCreateInstanceToken: %v", err)
	}

	runnerAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	runnerBaseURL, err := openURLForListenAddr(runnerAddr)
	if err != nil {
		t.Fatalf("openURLForListenAddr(runner): %v", err)
	}

	secretaryAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	secretaryBaseURL, err := openURLForListenAddr(secretaryAddr)
	if err != nil {
		t.Fatalf("openURLForListenAddr(secretary): %v", err)
	}

	if err := ensureRunnerd(ctx, runnerBaseURL, instanceToken, helperSpawnArgs("runnerd", dataDir, runnerAddr, secretaryAddr, runnerBaseURL)); err != nil {
		t.Fatalf("ensureRunnerd(first): %v", err)
	}
	runnerPID1 := mustDaemonPID(t, runnerBaseURL, instanceToken)
	t.Cleanup(func() { _ = killPID(runnerPID1) })

	if err := ensureRunnerd(ctx, runnerBaseURL, instanceToken, helperSpawnArgs("runnerd", dataDir, runnerAddr, secretaryAddr, runnerBaseURL)); err != nil {
		t.Fatalf("ensureRunnerd(second): %v", err)
	}
	runnerPID2 := mustDaemonPID(t, runnerBaseURL, instanceToken)
	if runnerPID2 != runnerPID1 {
		t.Fatalf("runnerd pid changed on second ensure: got=%d want=%d", runnerPID2, runnerPID1)
	}

	if err := ensureSecretaryd(ctx, secretaryBaseURL, instanceToken, helperSpawnArgs("secretaryd", dataDir, runnerAddr, secretaryAddr, runnerBaseURL)); err != nil {
		t.Fatalf("ensureSecretaryd(first): %v", err)
	}
	secretaryPID1 := mustDaemonPID(t, secretaryBaseURL, instanceToken)
	t.Cleanup(func() { _ = killPID(secretaryPID1) })

	if err := ensureSecretaryd(ctx, secretaryBaseURL, instanceToken, helperSpawnArgs("secretaryd", dataDir, runnerAddr, secretaryAddr, runnerBaseURL)); err != nil {
		t.Fatalf("ensureSecretaryd(second): %v", err)
	}
	secretaryPID2 := mustDaemonPID(t, secretaryBaseURL, instanceToken)
	if secretaryPID2 != secretaryPID1 {
		t.Fatalf("secretaryd pid changed on second ensure: got=%d want=%d", secretaryPID2, secretaryPID1)
	}
}

func TestDaemonSingleInstanceGuard_SecondStartExits(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Best-effort: process + signal behavior on Windows can be flaky under `go test`.
		t.Skip("skip on windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	dataDir := t.TempDir()
	cfg, err := config.Load(dataDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	instanceToken, err := daemon.LoadOrCreateInstanceToken(cfg.Paths.DataDir)
	if err != nil {
		t.Fatalf("LoadOrCreateInstanceToken: %v", err)
	}

	runnerAddr := fmt.Sprintf("127.0.0.1:%d", pickPort(t))
	runnerBaseURL, err := openURLForListenAddr(runnerAddr)
	if err != nil {
		t.Fatalf("openURLForListenAddr(runner): %v", err)
	}

	if err := ensureRunnerd(ctx, runnerBaseURL, instanceToken, helperSpawnArgs("runnerd", dataDir, runnerAddr, "", runnerBaseURL)); err != nil {
		t.Fatalf("ensureRunnerd(first): %v", err)
	}
	runnerPID := mustDaemonPID(t, runnerBaseURL, instanceToken)
	t.Cleanup(func() { _ = killPID(runnerPID) })

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(exe, helperSpawnArgs("runnerd", dataDir, runnerAddr, "", runnerBaseURL)...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("start second runnerd: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second runnerd exit err: %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("second runnerd did not exit quickly (single-instance guard likely missing)")
	}

	// Ensure original daemon is still serving and has the same pid.
	if got := mustDaemonPID(t, runnerBaseURL, instanceToken); got != runnerPID {
		t.Fatalf("runnerd pid changed after second start: got=%d want=%d", got, runnerPID)
	}
}

func helperSpawnArgs(mode string, dataDir string, runnerAddr string, secretaryAddr string, runnerBaseURL string) []string {
	args := []string{
		"-test.run=TestControlCCXDaemonHelperProcess",
		"--",
		"controlccx-daemon-helper",
		mode,
		"--data-dir", strings.TrimSpace(dataDir),
	}
	if strings.TrimSpace(runnerAddr) != "" {
		args = append(args, "--runnerd-addr", strings.TrimSpace(runnerAddr))
	}
	if strings.TrimSpace(secretaryAddr) != "" {
		args = append(args, "--secretaryd-addr", strings.TrimSpace(secretaryAddr))
	}
	if strings.TrimSpace(runnerBaseURL) != "" {
		args = append(args, "--runner-base-url", strings.TrimSpace(runnerBaseURL))
	}
	return args
}

func TestControlCCXDaemonHelperProcess(t *testing.T) {
	args := flag.Args()
	if len(args) < 2 || args[0] != "controlccx-daemon-helper" {
		return
	}

	mode := strings.TrimSpace(args[1])
	fs := flag.NewFlagSet("controlccx-daemon-helper", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataDir := fs.String("data-dir", "", "data dir")
	runnerAddr := fs.String("runnerd-addr", "", "runner addr")
	secretaryAddr := fs.String("secretaryd-addr", "", "secretary addr")
	runnerBaseURL := fs.String("runner-base-url", "", "runner base url")
	if err := fs.Parse(args[2:]); err != nil {
		os.Exit(2)
	}

	cfg, err := config.Load(strings.TrimSpace(*dataDir))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	switch mode {
	case "runnerd":
		if err := runRunnerd(cfg, strings.TrimSpace(*runnerAddr)); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}
	case "secretaryd":
		if err := runSecretaryd(cfg, strings.TrimSpace(*secretaryAddr), strings.TrimSpace(*runnerBaseURL)); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %q\n", mode)
		os.Exit(2)
	}

	os.Exit(0)
}

func pickPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen :0: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr == nil || addr.Port <= 0 {
		t.Fatalf("unexpected addr: %v", ln.Addr())
	}
	return addr.Port
}

func mustDaemonPID(t *testing.T, baseURL string, token string) int {
	t.Helper()

	type payload struct {
		PID int `json:"pid"`
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(daemon.InstanceTokenHeader, token)

	res, err := (&http.Client{Timeout: 800 * time.Millisecond}).Do(req)
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", res.StatusCode)
	}
	var out payload
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.PID <= 0 {
		t.Fatalf("unexpected pid: %d", out.PID)
	}
	return out.PID
}

func killPID(pid int) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
