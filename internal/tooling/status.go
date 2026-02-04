package tooling

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type ToolStatus struct {
	ID           string `json:"id"`
	Driver       Driver `json:"driver"`
	Command      string `json:"command"`
	Available    bool   `json:"available"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (s *Service) Status() []ToolStatus {
	if s == nil {
		return nil
	}
	tools := s.List()
	if len(tools) == 0 {
		return nil
	}
	pathEnv := os.Getenv("PATH")

	out := make([]ToolStatus, 0, len(tools))
	for _, t := range tools {
		out = append(out, statusForTool(t, pathEnv))
	}
	return out
}

func statusForTool(t Tool, pathEnv string) ToolStatus {
	st := ToolStatus{
		ID:      t.ID,
		Driver:  t.Driver,
		Command: t.Command,
	}

	cmd := strings.TrimSpace(t.Command)
	if cmd == "" {
		st.Error = "missing command"
		return st
	}

	resolved, err := lookPathWithEnv(cmd, pathEnv)
	if err != nil {
		st.Error = err.Error()
		return st
	}

	st.Available = true
	st.ResolvedPath = resolved
	return st
}

func lookPathWithEnv(file string, pathEnv string) (string, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return "", errors.New("empty command")
	}

	if hasPathSeparator(file) {
		return resolveDirect(file)
	}

	dirs := filepath.SplitList(pathEnv)
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(file))
		exts := pathextList()

		if ext != "" {
			return searchWindows(dirs, file, nil)
		}
		return searchWindows(dirs, file, exts)
	}

	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, file)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("not found on PATH")
}

func hasPathSeparator(p string) bool {
	return strings.Contains(p, "/") || strings.Contains(p, "\\")
}

func resolveDirect(p string) (string, error) {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(p))
		if ext != "" {
			if isExecutableFile(p) {
				return p, nil
			}
			return "", errors.New("not found")
		}
		exts := pathextList()
		for _, e := range exts {
			candidate := p + e
			if isExecutableFile(candidate) {
				return candidate, nil
			}
		}
		if isExecutableFile(p) {
			return p, nil
		}
		return "", errors.New("not found")
	}

	if isExecutableFile(p) {
		return p, nil
	}
	return "", errors.New("not found")
}

func pathextList() []string {
	raw := strings.TrimSpace(os.Getenv("PATHEXT"))
	if raw == "" {
		raw = ".COM;.EXE;.BAT;.CMD"
	}
	parts := strings.Split(raw, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		out = append(out, strings.ToLower(p))
	}
	return out
}

func searchWindows(dirs []string, file string, exts []string) (string, error) {
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		base := filepath.Join(dir, file)
		if len(exts) == 0 {
			if isExecutableFile(base) {
				return base, nil
			}
			continue
		}
		for _, e := range exts {
			if e == "" {
				continue
			}
			candidate := base + e
			if isExecutableFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", errors.New("not found on PATH")
}

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info == nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
