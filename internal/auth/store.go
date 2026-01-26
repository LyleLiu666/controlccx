package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type Secrets struct {
	AnthropicBaseURL   string `json:"anthropic_base_url,omitempty"`
	AnthropicAPIKey    string `json:"anthropic_api_key,omitempty"`
	AnthropicAuthToken string `json:"anthropic_auth_token,omitempty"`
	AnthropicModel     string `json:"anthropic_model,omitempty"`
	AnthropicSmallFastModel string `json:"anthropic_small_fast_model,omitempty"`
	OpenAIAPIKey       string `json:"openai_api_key,omitempty"`
	CodexModel         string `json:"codex_model,omitempty"`
	CodexReasoningEffort string `json:"codex_reasoning_effort,omitempty"`
}

type Patch struct {
	AnthropicBaseURL   *string `json:"anthropic_base_url,omitempty"`
	AnthropicAPIKey    *string `json:"anthropic_api_key,omitempty"`
	AnthropicAuthToken *string `json:"anthropic_auth_token,omitempty"`
	AnthropicModel     *string `json:"anthropic_model,omitempty"`
	AnthropicSmallFastModel *string `json:"anthropic_small_fast_model,omitempty"`
	OpenAIAPIKey       *string `json:"openai_api_key,omitempty"`
	CodexModel         *string `json:"codex_model,omitempty"`
	CodexReasoningEffort *string `json:"codex_reasoning_effort,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	secrets Secrets
}

func Load(path string) (*Store, error) {
	s := &Store{path: filepath.Clean(path)}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Get() Secrets {
	if s == nil {
		return Secrets{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.secrets
}

func (s *Store) ApplyPatch(p Patch) (Secrets, error) {
	if s == nil {
		return Secrets{}, errors.New("auth: store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	apply := func(dst *string, v *string) {
		if v == nil {
			return
		}
		*dst = strings.TrimSpace(*v)
	}
	apply(&s.secrets.AnthropicBaseURL, p.AnthropicBaseURL)
	apply(&s.secrets.AnthropicAPIKey, p.AnthropicAPIKey)
	apply(&s.secrets.AnthropicAuthToken, p.AnthropicAuthToken)
	apply(&s.secrets.AnthropicModel, p.AnthropicModel)
	apply(&s.secrets.AnthropicSmallFastModel, p.AnthropicSmallFastModel)
	apply(&s.secrets.OpenAIAPIKey, p.OpenAIAPIKey)
	apply(&s.secrets.CodexModel, p.CodexModel)
	apply(&s.secrets.CodexReasoningEffort, p.CodexReasoningEffort)

	if err := s.saveLocked(); err != nil {
		return Secrets{}, err
	}
	return s.secrets, nil
}

func (s *Store) reload() error {
	if s.path == "" {
		return errors.New("auth: path is required")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("auth: read %s: %w", s.path, err)
	}
	var v Secrets
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("auth: parse %s: %w", s.path, err)
	}
	s.secrets = v
	return nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return errors.New("auth: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("auth: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s.secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: marshal: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(s.path, data, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".controlccx-auth-*")
	if err != nil {
		return fmt.Errorf("auth: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if runtime.GOOS != "windows" {
		_ = tmp.Chmod(perm)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("auth: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("auth: close temp: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("auth: rename temp: %w", err)
	}
	return nil
}
