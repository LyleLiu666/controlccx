package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func currentExecutableBinaryStampBestEffort() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return binaryStampForPathBestEffort(exe)
}

func binaryStampForPathBestEffort(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	resolved := path
	if real, err := filepath.EvalSymlinks(path); err == nil && strings.TrimSpace(real) != "" {
		resolved = real
	}
	if abs, err := filepath.Abs(resolved); err == nil && strings.TrimSpace(abs) != "" {
		resolved = abs
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return ""
	}
	modNanos := info.ModTime().UTC().UnixNano()
	return fmt.Sprintf("%s|%d|%d", filepath.Clean(resolved), info.Size(), modNanos)
}
