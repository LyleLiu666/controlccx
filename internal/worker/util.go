package worker

import (
	"errors"
	"io"
	"os/exec"
	"strings"
)

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}

func exitCode(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

