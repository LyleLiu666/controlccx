package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrAlreadyRunning = errors.New("daemon: already running")

type SingleInstanceLock struct {
	f    *os.File
	path string
}

type lockMeta struct {
	Name string `json:"name"`
	PID  int    `json:"pid"`
	Addr string `json:"addr,omitempty"`
	TsMs int64  `json:"ts_ms"`
}

func AcquireSingleInstanceLock(dataDir string, name string, addr string) (*SingleInstanceLock, error) {
	dataDir = strings.TrimSpace(dataDir)
	name = strings.TrimSpace(name)
	addr = strings.TrimSpace(addr)

	if dataDir == "" {
		return nil, errors.New("daemon: data dir is required")
	}
	if name == "" {
		return nil, errors.New("daemon: lock name is required")
	}
	if err := os.MkdirAll(filepath.Clean(dataDir), 0o755); err != nil {
		return nil, fmt.Errorf("daemon: ensure data dir: %w", err)
	}

	p := filepath.Join(filepath.Clean(dataDir), fmt.Sprintf("%s.lock", name))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("daemon: open lock file: %w", err)
	}

	if err := tryLockFileNonBlocking(f); err != nil {
		_ = f.Close()
		if errors.Is(err, ErrAlreadyRunning) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("daemon: lock %s: %w", p, err)
	}

	meta := lockMeta{
		Name: name,
		PID:  os.Getpid(),
		Addr: addr,
		TsMs: time.Now().UTC().UnixMilli(),
	}
	if err := writeLockMetaBestEffort(f, meta); err != nil {
		// Not fatal: lock is held; metadata is only for debugging.
	}

	return &SingleInstanceLock{f: f, path: p}, nil
}

func (l *SingleInstanceLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func writeLockMetaBestEffort(f *os.File, meta lockMeta) error {
	if f == nil {
		return errors.New("daemon: lock file is nil")
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Best-effort: ignore errors; this is only for debugging.
	_, _ = f.Seek(0, 0)
	_ = f.Truncate(0)
	_, _ = f.Write(data)
	_ = f.Sync()
	return nil
}
