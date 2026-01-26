package worker

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
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
	return err == io.EOF
}

func formatReadError(err error) error {
	return fmt.Errorf("worker: read output: %w", err)
}

