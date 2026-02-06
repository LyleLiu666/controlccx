package daemon

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func TestAcquireSingleInstanceLock_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()

	lock, err := AcquireSingleInstanceLock(dir, "runnerd", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("AcquireSingleInstanceLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	exit := runLockHelper(t, dir)
	if exit != 42 {
		t.Fatalf("expected helper exit=42 (already running), got %d", exit)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	exit = runLockHelper(t, dir)
	if exit != 0 {
		t.Fatalf("expected helper exit=0 after release, got %d", exit)
	}
}

func runLockHelper(t *testing.T, dir string) int {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	cmd := exec.Command(exe, "-test.run=TestLockHelperProcess", "--", dir)
	cmd.Env = append(os.Environ(), "CONTROLCCX_LOCK_HELPER=1")
	err = cmd.Run()

	var exitErr *exec.ExitError
	if err == nil {
		return 0
	}
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	t.Fatalf("helper failed unexpectedly: %v", err)
	return -1
}

func TestLockHelperProcess(t *testing.T) {
	if os.Getenv("CONTROLCCX_LOCK_HELPER") != "1" {
		return
	}
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "missing lock dir arg")
		os.Exit(2)
	}

	_, err := AcquireSingleInstanceLock(args[0], "runnerd", "127.0.0.1:0")
	if err == nil {
		os.Exit(0)
	}
	if errors.Is(err, ErrAlreadyRunning) {
		os.Exit(42)
	}
	fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(2)
}
