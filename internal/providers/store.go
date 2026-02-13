package providers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"controlccx/internal/auth"
)

type ActiveSelection struct {
	Claude    string `json:"claude,omitempty"`
	Codex     string `json:"codex,omitempty"`
	Secretary string `json:"secretary,omitempty"`
}

type Targets struct {
	Claude    ClaudeTarget    `json:"claude,omitempty"`
	Codex     CodexTarget     `json:"codex,omitempty"`
	Secretary SecretaryTarget `json:"secretary,omitempty"`
}

type ClaudeTarget struct {
	BaseURL        string `json:"base_url,omitempty"`
	APIKey         string `json:"api_key,omitempty"`
	AuthToken      string `json:"auth_token,omitempty"`
	Model          string `json:"model,omitempty"`
	SmallFastModel string `json:"small_fast_model,omitempty"`
}

type CodexTarget struct {
	BaseURL         string `json:"base_url,omitempty"`
	APIKey          string `json:"api_key,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type SecretaryTarget struct {
	Backend string `json:"backend,omitempty"` // simple-http | openai-chat

	// SimpleHTTP config is used by the Secretary backend. This is intentionally separate from the Claude Code target
	// so Secretary can use an independent auth set.
	SimpleHTTP SecretarySimpleHTTP `json:"simple_http,omitempty"`

	OpenAIChat SecretaryOpenAIChat `json:"openai_chat,omitempty"`
}

type SecretarySimpleHTTP struct {
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	AuthToken string `json:"auth_token,omitempty"`
	Model     string `json:"model,omitempty"`
}

type SecretaryOpenAIChat struct {
	BaseURL string `json:"base_url,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Model   string `json:"model,omitempty"`
}

type SyncLive struct {
	Claude    bool `json:"claude,omitempty"`
	Codex     bool `json:"codex,omitempty"`
	Secretary bool `json:"secretary,omitempty"`
}

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tool      string    `json:"tool,omitempty"`
	Targets   Targets   `json:"targets,omitempty"`
	SyncLive  SyncLive  `json:"sync_live,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type fileModel struct {
	Profiles []Profile       `json:"profiles,omitempty"`
	Active   ActiveSelection `json:"active,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	m    fileModel
}

func NewStore(dataDir string) (*Store, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, errors.New("providers: data dir is required")
	}
	dataDir = filepath.Clean(dataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("providers: ensure data dir: %w", err)
	}

	s := &Store{path: filepath.Join(dataDir, "providers.json")}
	if err := s.reloadLocked(); err != nil {
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

func (s *Store) Reload() error {
	if s == nil {
		return errors.New("providers: store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked()
}

func (s *Store) Active() ActiveSelection {
	if s == nil {
		return ActiveSelection{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m.Active
}

func (s *Store) Profiles() []Profile {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneProfiles(s.m.Profiles)
}

func (s *Store) MaskedProfiles() []Profile {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := cloneProfiles(s.m.Profiles)
	for i := range out {
		out[i] = MaskProfile(out[i])
	}
	return out
}

func (s *Store) Get(id string) (Profile, bool) {
	if s == nil {
		return Profile{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Profile{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.m.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

func (s *Store) Upsert(p Profile) (Profile, error) {
	if s == nil {
		return Profile{}, errors.New("providers: store is nil")
	}

	p = normalizeProfile(p)
	if strings.TrimSpace(p.Name) == "" {
		return Profile{}, errors.New("providers: name is required")
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	if hasProfileNameLocked(s.m.Profiles, p.Name, p.ID) {
		return Profile{}, errors.New("providers: name already exists")
	}

	if strings.TrimSpace(p.ID) == "" {
		id, err := newID()
		if err != nil {
			return Profile{}, err
		}
		p.ID = id
		p.CreatedAt = now
		p.UpdatedAt = now
		s.m.Profiles = append(s.m.Profiles, p)
		if err := s.saveLocked(); err != nil {
			return Profile{}, err
		}
		return p, nil
	}

	for i := range s.m.Profiles {
		if s.m.Profiles[i].ID != p.ID {
			continue
		}
		p.CreatedAt = s.m.Profiles[i].CreatedAt
		p.UpdatedAt = now
		s.m.Profiles[i] = p
		if err := s.saveLocked(); err != nil {
			return Profile{}, err
		}
		return p, nil
	}

	// Unknown ID: treat as create (preserve the provided id).
	p.CreatedAt = now
	p.UpdatedAt = now
	s.m.Profiles = append(s.m.Profiles, p)
	if err := s.saveLocked(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (s *Store) Delete(id string) error {
	if s == nil {
		return errors.New("providers: store is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("providers: id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i := range s.m.Profiles {
		if s.m.Profiles[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	s.m.Profiles = append(s.m.Profiles[:idx], s.m.Profiles[idx+1:]...)
	if s.m.Active.Claude == id {
		s.m.Active.Claude = ""
	}
	if s.m.Active.Codex == id {
		s.m.Active.Codex = ""
	}
	if s.m.Active.Secretary == id {
		s.m.Active.Secretary = ""
	}
	return s.saveLocked()
}

func (s *Store) Duplicate(id string, newName string) (Profile, error) {
	if s == nil {
		return Profile{}, errors.New("providers: store is nil")
	}
	id = strings.TrimSpace(id)
	newName = strings.TrimSpace(newName)
	if id == "" {
		return Profile{}, errors.New("providers: id is required")
	}
	if newName == "" {
		return Profile{}, errors.New("providers: new name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var src Profile
	ok := false
	for _, p := range s.m.Profiles {
		if p.ID == id {
			src = p
			ok = true
			break
		}
	}
	if !ok {
		return Profile{}, errors.New("providers: profile not found")
	}
	if hasProfileNameLocked(s.m.Profiles, newName, "") {
		return Profile{}, errors.New("providers: name already exists")
	}

	newID, err := newID()
	if err != nil {
		return Profile{}, err
	}
	now := time.Now().UTC()
	dup := src
	dup.ID = newID
	dup.Name = newName
	dup.CreatedAt = now
	dup.UpdatedAt = now
	s.m.Profiles = append(s.m.Profiles, dup)
	if err := s.saveLocked(); err != nil {
		return Profile{}, err
	}
	return dup, nil
}

func (s *Store) Reorder(ids []string) error {
	if s == nil {
		return errors.New("providers: store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) != len(s.m.Profiles) {
		return errors.New("providers: reorder: ids length mismatch")
	}

	seen := make(map[string]bool, len(ids))
	want := make(map[string]Profile, len(s.m.Profiles))
	for _, p := range s.m.Profiles {
		want[p.ID] = p
	}

	var next []Profile
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("providers: reorder: empty id")
		}
		if seen[id] {
			return errors.New("providers: reorder: duplicate id")
		}
		seen[id] = true
		p, ok := want[id]
		if !ok {
			return errors.New("providers: reorder: unknown id")
		}
		next = append(next, p)
	}
	s.m.Profiles = next
	return s.saveLocked()
}

func (s *Store) SetActive(target string, profileID string) error {
	if s == nil {
		return errors.New("providers: store is nil")
	}
	target = strings.ToLower(strings.TrimSpace(target))
	profileID = strings.TrimSpace(profileID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if profileID != "" && !hasProfileID(s.m.Profiles, profileID) {
		return errors.New("providers: active: profile not found")
	}

	switch target {
	case "claude":
		s.m.Active.Claude = profileID
	case "codex":
		s.m.Active.Codex = profileID
	case "secretary":
		s.m.Active.Secretary = profileID
	default:
		return errors.New("providers: active: unknown target")
	}
	return s.saveLocked()
}

func MaskProfile(p Profile) Profile {
	p.Targets.Claude.APIKey = auth.MaskSecret(p.Targets.Claude.APIKey)
	p.Targets.Claude.AuthToken = auth.MaskSecret(p.Targets.Claude.AuthToken)
	p.Targets.Codex.APIKey = auth.MaskSecret(p.Targets.Codex.APIKey)
	p.Targets.Secretary.SimpleHTTP.APIKey = auth.MaskSecret(p.Targets.Secretary.SimpleHTTP.APIKey)
	p.Targets.Secretary.SimpleHTTP.AuthToken = auth.MaskSecret(p.Targets.Secretary.SimpleHTTP.AuthToken)
	p.Targets.Secretary.OpenAIChat.APIKey = auth.MaskSecret(p.Targets.Secretary.OpenAIChat.APIKey)
	return p
}

func normalizeProfile(p Profile) Profile {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Tool = normalizeProfileTool(p.Tool)

	p.Targets.Claude.BaseURL = strings.TrimSpace(p.Targets.Claude.BaseURL)
	p.Targets.Claude.APIKey = strings.TrimSpace(p.Targets.Claude.APIKey)
	p.Targets.Claude.AuthToken = strings.TrimSpace(p.Targets.Claude.AuthToken)
	p.Targets.Claude.Model = strings.TrimSpace(p.Targets.Claude.Model)
	p.Targets.Claude.SmallFastModel = strings.TrimSpace(p.Targets.Claude.SmallFastModel)

	p.Targets.Codex.BaseURL = strings.TrimSpace(p.Targets.Codex.BaseURL)
	p.Targets.Codex.APIKey = strings.TrimSpace(p.Targets.Codex.APIKey)
	p.Targets.Codex.Model = strings.TrimSpace(p.Targets.Codex.Model)
	p.Targets.Codex.ReasoningEffort = strings.TrimSpace(p.Targets.Codex.ReasoningEffort)

	p.Targets.Secretary.Backend = strings.TrimSpace(p.Targets.Secretary.Backend)
	p.Targets.Secretary.SimpleHTTP.BaseURL = strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.BaseURL)
	p.Targets.Secretary.SimpleHTTP.APIKey = strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.APIKey)
	p.Targets.Secretary.SimpleHTTP.AuthToken = strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.AuthToken)
	p.Targets.Secretary.SimpleHTTP.Model = strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.Model)
	p.Targets.Secretary.OpenAIChat.BaseURL = strings.TrimSpace(p.Targets.Secretary.OpenAIChat.BaseURL)
	p.Targets.Secretary.OpenAIChat.APIKey = strings.TrimSpace(p.Targets.Secretary.OpenAIChat.APIKey)
	p.Targets.Secretary.OpenAIChat.Model = strings.TrimSpace(p.Targets.Secretary.OpenAIChat.Model)

	backend := strings.ToLower(strings.TrimSpace(p.Targets.Secretary.Backend))
	switch backend {
	case "", "simple-http", "openai-chat":
	default:
		backend = "simple-http"
	}
	if backend == "" {
		if strings.TrimSpace(p.Targets.Secretary.OpenAIChat.BaseURL) != "" ||
			strings.TrimSpace(p.Targets.Secretary.OpenAIChat.APIKey) != "" ||
			strings.TrimSpace(p.Targets.Secretary.OpenAIChat.Model) != "" {
			backend = "openai-chat"
		} else if p.Tool == "secretary" ||
			strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.BaseURL) != "" ||
			strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.APIKey) != "" ||
			strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.AuthToken) != "" ||
			strings.TrimSpace(p.Targets.Secretary.SimpleHTTP.Model) != "" {
			backend = "simple-http"
		}
	}
	p.Targets.Secretary.Backend = backend

	if p.Tool == "" {
		p.Tool = inferProfileTool(p.Targets)
	}
	if p.Tool == "" {
		p.Tool = "claude"
	}
	return p
}

func normalizeProfileTool(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	case "secretary":
		return "secretary"
	default:
		return ""
	}
}

func inferProfileTool(targets Targets) string {
	if hasClaudeTargetData(targets.Claude) {
		return "claude"
	}
	if hasCodexTargetData(targets.Codex) {
		return "codex"
	}
	if hasSecretaryTargetData(targets.Secretary) {
		return "secretary"
	}
	return ""
}

func hasClaudeTargetData(t ClaudeTarget) bool {
	return strings.TrimSpace(t.BaseURL) != "" ||
		strings.TrimSpace(t.APIKey) != "" ||
		strings.TrimSpace(t.AuthToken) != "" ||
		strings.TrimSpace(t.Model) != "" ||
		strings.TrimSpace(t.SmallFastModel) != ""
}

func hasCodexTargetData(t CodexTarget) bool {
	return strings.TrimSpace(t.BaseURL) != "" ||
		strings.TrimSpace(t.APIKey) != "" ||
		strings.TrimSpace(t.Model) != "" ||
		strings.TrimSpace(t.ReasoningEffort) != ""
}

func hasSecretaryTargetData(t SecretaryTarget) bool {
	if strings.TrimSpace(t.Backend) != "" {
		return true
	}
	return strings.TrimSpace(t.SimpleHTTP.BaseURL) != "" ||
		strings.TrimSpace(t.SimpleHTTP.APIKey) != "" ||
		strings.TrimSpace(t.SimpleHTTP.AuthToken) != "" ||
		strings.TrimSpace(t.SimpleHTTP.Model) != "" ||
		strings.TrimSpace(t.OpenAIChat.BaseURL) != "" ||
		strings.TrimSpace(t.OpenAIChat.APIKey) != "" ||
		strings.TrimSpace(t.OpenAIChat.Model) != ""
}

func cloneProfiles(in []Profile) []Profile {
	if len(in) == 0 {
		return nil
	}
	out := make([]Profile, len(in))
	copy(out, in)
	return out
}

func hasProfileID(ps []Profile, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, p := range ps {
		if p.ID == id {
			return true
		}
	}
	return false
}

func hasProfileNameLocked(ps []Profile, name string, exceptID string) bool {
	nameKey := normalizedProfileName(name)
	if nameKey == "" {
		return false
	}
	exceptID = strings.TrimSpace(exceptID)
	for _, p := range ps {
		if exceptID != "" && p.ID == exceptID {
			continue
		}
		if normalizedProfileName(p.Name) == nameKey {
			return true
		}
	}
	return false
}

func normalizedProfileName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (s *Store) reloadLocked() error {
	if s.path == "" {
		return errors.New("providers: path is required")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("providers: read %s: %w", s.path, err)
	}
	var v fileModel
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("providers: parse %s: %w", s.path, err)
	}
	// Best-effort normalization.
	for i := range v.Profiles {
		v.Profiles[i] = normalizeProfile(v.Profiles[i])
	}
	s.m = v
	return nil
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return errors.New("providers: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("providers: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return fmt.Errorf("providers: marshal: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(s.path, data, 0o600)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".controlccx-providers-*")
	if err != nil {
		return fmt.Errorf("providers: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if runtime.GOOS != "windows" {
		_ = tmp.Chmod(perm)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("providers: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("providers: close temp: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("providers: rename temp: %w", err)
	}
	return nil
}

func newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("providers: random id: %w", err)
	}
	// Stable enough for local profiles; avoids bringing in a uuid dependency.
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
