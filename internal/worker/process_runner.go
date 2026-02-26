package worker

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// osProcessRunner implements ProcessRunner using the standard os/exec package.
type osProcessRunner struct{}

func newOSProcessRunner() *osProcessRunner {
	return &osProcessRunner{}
}

func (r *osProcessRunner) Spawn(ctx context.Context, opts SpawnOpts) (ManagedRun, error) {
	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env

	// Set up pipes for standard streams.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		// Close pipes if start fails to prevent leaks
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}

	return &osManagedRun{
		cmd:    cmd,
		cancel: nil, // we rely on the parent ctx for cancellation if needed, or explicitly via Cancel()
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

// osManagedRun implements ManagedRun, wrapping an active *exec.Cmd.
type osManagedRun struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc

	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (r *osManagedRun) PID() int {
	if r.cmd != nil && r.cmd.Process != nil {
		return r.cmd.Process.Pid
	}
	return 0
}

func (r *osManagedRun) Stdout() io.Reader {
	return r.stdout
}

func (r *osManagedRun) Stderr() io.Reader {
	return r.stderr
}

func (r *osManagedRun) Stdin() io.WriteCloser {
	return r.stdin
}

func (r *osManagedRun) Wait() (int, error) {
	err := r.cmd.Wait()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return -1, err
	}
	// exit code 0
	return r.cmd.ProcessState.ExitCode(), nil
}

func (r *osManagedRun) Cancel() error {
	if r.cancel != nil {
		r.cancel()
		return nil
	}
	if r.cmd.Process != nil {
		// Attempt to kill the process if we don't have a cancel func,
		// though typically exec.CommandContext handles this natively.
		return r.cmd.Process.Signal(os.Interrupt)
	}
	return nil
}
