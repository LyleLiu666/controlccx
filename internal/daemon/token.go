package daemon

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ProtocolVersion     = 1
	InstanceTokenHeader = "X-ControlCCX-Token"
)

func InstanceTokenPath(dataDir string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return "", errors.New("daemon: data dir is required")
	}
	return filepath.Join(filepath.Clean(dataDir), "instance.token"), nil
}

func LoadOrCreateInstanceToken(dataDir string) (string, error) {
	path, err := InstanceTokenPath(dataDir)
	if err != nil {
		return "", err
	}

	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("daemon: read instance token: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("daemon: generate instance token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("daemon: ensure data dir: %w", err)
	}
	if err := writeFileAtomic(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func HasValidInstanceToken(r httpRequestHeader, token string) bool {
	if r == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	got := strings.TrimSpace(r.Get(InstanceTokenHeader))
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

type httpRequestHeader interface {
	Get(key string) string
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".controlccx-instance-*")
	if err != nil {
		return fmt.Errorf("daemon: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if runtime.GOOS != "windows" {
		_ = tmp.Chmod(perm)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("daemon: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("daemon: close temp: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("daemon: rename temp: %w", err)
	}
	return nil
}
