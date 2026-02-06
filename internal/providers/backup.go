package providers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultBackupKeep = 10

func CreateRotatingBackup(srcPath string, backupDir string, keep int) (string, error) {
	srcPath = strings.TrimSpace(srcPath)
	backupDir = strings.TrimSpace(backupDir)
	if srcPath == "" {
		return "", errors.New("providers: backup: src path is required")
	}
	if backupDir == "" {
		return "", errors.New("providers: backup: backup dir is required")
	}
	if keep <= 0 {
		return "", errors.New("providers: backup: keep must be > 0")
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("providers: backup: read: %w", err)
	}

	if err := os.MkdirAll(filepath.Clean(backupDir), 0o700); err != nil {
		return "", fmt.Errorf("providers: backup: mkdir: %w", err)
	}

	suffix, err := randomHex(8)
	if err != nil {
		return "", err
	}
	ext := filepath.Ext(srcPath)
	if strings.TrimSpace(ext) == "" {
		ext = ".bak"
	}
	// Include fractional seconds so lexicographic order matches creation order even under fast successive backups.
	name := fmt.Sprintf("%s-%s%s", time.Now().UTC().Format("20060102-150405.000000000"), suffix, ext)
	dest := filepath.Join(filepath.Clean(backupDir), name)

	if err := writeFileAtomic(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("providers: backup: write: %w", err)
	}

	if err := rotateBackups(backupDir, keep); err != nil {
		return "", err
	}
	return dest, nil
}

func rotateBackups(backupDir string, keep int) error {
	entries, err := os.ReadDir(filepath.Clean(backupDir))
	if err != nil {
		return fmt.Errorf("providers: backup: readdir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e == nil || e.IsDir() {
			continue
		}
		n := strings.TrimSpace(e.Name())
		if n == "" {
			continue
		}
		names = append(names, n)
	}

	sort.Strings(names)
	// Newest first (timestamped prefixes sort ascending).
	for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
		names[i], names[j] = names[j], names[i]
	}

	for i := keep; i < len(names); i++ {
		_ = os.Remove(filepath.Join(filepath.Clean(backupDir), names[i]))
	}
	return nil
}

func randomHex(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", errors.New("providers: backup: nBytes must be > 0")
	}
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("providers: backup: random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
