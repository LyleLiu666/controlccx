package worker

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	defaultJSONLineReaderSize = 64 * 1024
	defaultJSONLineMaxBytes   = 10 * 1024 * 1024
)

func newLineReader(r io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(r, defaultJSONLineReaderSize)
}

func readLineWithLimit(reader *bufio.Reader, maxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultJSONLineMaxBytes
	}

	var buf []byte
	for {
		line, isPrefix, err := reader.ReadLine()
		if err != nil {
			return nil, false, err
		}
		if len(buf)+len(line) > maxBytes {
			// Drain the rest of the line if needed.
			for isPrefix {
				_, isPrefix, err = reader.ReadLine()
				if err != nil {
					return nil, true, err
				}
			}
			return nil, true, nil
		}
		buf = append(buf, line...)
		if !isPrefix {
			break
		}
	}
	return bytes.TrimSpace(buf), false, nil
}

func isEOF(err error) bool {
	if err == nil {
		return false
	}
	// exec.Cmd may close StdoutPipe/StderrPipe once Wait() returns. Depending on timing,
	// readers can observe os.ErrClosed instead of io.EOF. Treat both as EOF to avoid
	// dropping the last output lines.
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, os.ErrClosed) {
		return true
	}
	return false
}

func formatReadError(err error) error {
	return fmt.Errorf("worker: read output: %w", err)
}
