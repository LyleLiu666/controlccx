package tooling

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type Driver string

const (
	DriverClaudeCode Driver = "claude-code"
	DriverCodex      Driver = "codex"
	DriverExec       Driver = "exec"
)

type Tool struct {
	ID      string            `json:"id"`
	Driver  Driver            `json:"driver"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type fileModel struct {
	Tools []Tool `json:"tools"`
}

type Options struct {
	DataDir  string
	Defaults []Tool
}

type Service struct {
	mu       sync.RWMutex
	path     string
	defaults map[string]Tool
	custom   map[string]Tool
}

func NewService(opts Options) (*Service, error) {
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		return nil, errors.New("tooling: data dir is required")
	}
	dataDir = filepath.Clean(dataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("tooling: ensure data dir: %w", err)
	}

	s := &Service{
		path:     filepath.Join(dataDir, "tools.json"),
		defaults: make(map[string]Tool),
		custom:   make(map[string]Tool),
	}
	for _, d := range opts.Defaults {
		if strings.TrimSpace(d.ID) == "" {
			continue
		}
		s.defaults[d.ID] = normalizeTool(d)
	}
	if err := s.reloadLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) List() []Tool {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make(map[string]bool, len(s.defaults)+len(s.custom))
	for id := range s.defaults {
		ids[id] = true
	}
	for id := range s.custom {
		ids[id] = true
	}
	var out []Tool
	for id := range ids {
		if t, ok := s.custom[id]; ok {
			out = append(out, t)
			continue
		}
		if t, ok := s.defaults[id]; ok {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) Resolve(id string) (Tool, bool) {
	if s == nil {
		return Tool{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Tool{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if t, ok := s.custom[id]; ok {
		return t, true
	}
	t, ok := s.defaults[id]
	return t, ok
}

func (s *Service) Upsert(t Tool) error {
	if s == nil {
		return errors.New("tooling: service is nil")
	}
	t = normalizeTool(t)
	if err := validateTool(t); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.custom[t.ID] = t
	return s.saveLocked()
}

func (s *Service) Delete(id string) error {
	if s == nil {
		return errors.New("tooling: service is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("tooling: id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.custom, id)
	return s.saveLocked()
}

func (s *Service) reloadLocked() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("tooling: read %s: %w", s.path, err)
	}
	var v fileModel
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("tooling: parse %s: %w", s.path, err)
	}
	next := make(map[string]Tool)
	for _, t := range v.Tools {
		t = normalizeTool(t)
		if strings.TrimSpace(t.ID) == "" {
			continue
		}
		if err := validateTool(t); err != nil {
			// Ignore invalid persisted entries (best-effort).
			continue
		}
		next[t.ID] = t
	}
	s.custom = next
	return nil
}

func (s *Service) saveLocked() error {
	var ids []string
	for id := range s.custom {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var v fileModel
	for _, id := range ids {
		v.Tools = append(v.Tools, s.custom[id])
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("tooling: marshal: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(s.path, data, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".controlccx-tools-*")
	if err != nil {
		return fmt.Errorf("tooling: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if runtime.GOOS != "windows" {
		_ = tmp.Chmod(perm)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("tooling: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("tooling: close temp: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("tooling: rename temp: %w", err)
	}
	return nil
}

func normalizeTool(t Tool) Tool {
	t.ID = strings.TrimSpace(t.ID)
	t.Driver = Driver(strings.TrimSpace(string(t.Driver)))
	t.Command = strings.TrimSpace(t.Command)
	if len(t.Args) == 0 {
		t.Args = nil
	}
	if len(t.Env) == 0 {
		t.Env = nil
	} else {
		next := make(map[string]string, len(t.Env))
		for k, v := range t.Env {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			next[k] = strings.TrimSpace(v)
		}
		if len(next) == 0 {
			t.Env = nil
		} else {
			t.Env = next
		}
	}
	return t
}

func validateTool(t Tool) error {
	if t.ID == "" {
		return errors.New("tooling: id is required")
	}
	if !isSafeID(t.ID) {
		return fmt.Errorf("tooling: invalid id %q", t.ID)
	}
	switch t.Driver {
	case DriverClaudeCode, DriverCodex, DriverExec:
		// ok
	default:
		return fmt.Errorf("tooling: invalid driver %q", t.Driver)
	}
	if t.Command == "" {
		return errors.New("tooling: command is required")
	}
	return nil
}

func isSafeID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

