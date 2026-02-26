package worker

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOSProcessRunner_HappyPath(t *testing.T) {
	runner := newOSProcessRunner()

	// Should just echo "hello" and exit 0
	opts := SpawnOpts{
		Command: "echo",
		Args:    []string{"hello world"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	run, err := runner.Spawn(ctx, opts)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Read stdout
	outBuf := new(bytes.Buffer)
	_, err = io.Copy(outBuf, run.Stdout())
	if err != nil {
		t.Fatalf("Copy stdout failed: %v", err)
	}

	exitCode, err := run.Wait()
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	outStr := strings.TrimSpace(outBuf.String())
	if outStr != "hello world" {
		t.Fatalf("stdout = %q, want 'hello world'", outStr)
	}
}

func TestOSProcessRunner_CancelPropagation(t *testing.T) {
	runner := newOSProcessRunner()

	// process that sleeps for 10s
	opts := SpawnOpts{
		Command: "sleep",
		Args:    []string{"10"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	run, err := runner.Spawn(ctx, opts)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := run.Wait()
		errCh <- err
	}()

	// wait a tiny bit to ensure it started, then cancel the context
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error (signal: killed) from Wait()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked for too long after context cancellation")
	}
}

func TestOSProcessRunner_ExplicitCancel(t *testing.T) {
	runner := newOSProcessRunner()

	// another sleeping process
	opts := SpawnOpts{
		Command: "sleep",
		Args:    []string{"10"},
	}

	ctx, cancelParent := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelParent()

	run, err := runner.Spawn(ctx, opts)
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := run.Wait()
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)

	// explicitly cancel the process handle
	if err := run.Cancel(); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error after explicit Cancel()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked for too long after explicit Cancel()")
	}
}
