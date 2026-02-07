package providers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStoreCRUDAndMasking(t *testing.T) {
	dir := t.TempDir()

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p1, err := s.Upsert(Profile{
		Name: " Current ",
		Targets: Targets{
			Claude: ClaudeTarget{
				BaseURL:   " https://api.anthropic.com ",
				AuthToken: "  sk-ant-secret-1234567890  ",
				Model:     " claude-3-7-sonnet ",
			},
			Codex: CodexTarget{
				APIKey: " sk-openai-secret-abcdef ",
			},
		},
	})
	if err != nil {
		t.Fatalf("Upsert(create): %v", err)
	}
	if p1.ID == "" {
		t.Fatalf("expected id to be set")
	}
	if p1.Name != "Current" {
		t.Fatalf("expected trimmed name, got %q", p1.Name)
	}
	if p1.Targets.Claude.AuthToken != "sk-ant-secret-1234567890" {
		t.Fatalf("expected trimmed auth token, got %q", p1.Targets.Claude.AuthToken)
	}
	if p1.Targets.Codex.APIKey != "sk-openai-secret-abcdef" {
		t.Fatalf("expected trimmed codex api key, got %q", p1.Targets.Codex.APIKey)
	}
	if p1.CreatedAt.IsZero() || p1.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set")
	}

	if err := s.SetActive("claude", p1.ID); err != nil {
		t.Fatalf("SetActive(claude): %v", err)
	}
	if got := s.Active().Claude; got != p1.ID {
		t.Fatalf("active claude got=%q want=%q", got, p1.ID)
	}

	dup, err := s.Duplicate(p1.ID, "Copy")
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.ID == "" || dup.ID == p1.ID {
		t.Fatalf("expected dup id to differ")
	}
	if dup.Name != "Copy" {
		t.Fatalf("dup name got=%q", dup.Name)
	}
	if dup.Targets.Claude.AuthToken != p1.Targets.Claude.AuthToken {
		t.Fatalf("dup token mismatch")
	}

	if err := s.Reorder([]string{dup.ID, p1.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	ps := s.Profiles()
	if len(ps) != 2 || ps[0].ID != dup.ID || ps[1].ID != p1.ID {
		t.Fatalf("unexpected reorder result: %+v", ps)
	}

	masked := s.MaskedProfiles()
	if len(masked) != 2 {
		t.Fatalf("masked length=%d", len(masked))
	}
	for _, mp := range masked {
		if mp.Targets.Claude.AuthToken == p1.Targets.Claude.AuthToken {
			t.Fatalf("expected claude auth token to be masked")
		}
		if mp.Targets.Codex.APIKey == p1.Targets.Codex.APIKey {
			t.Fatalf("expected codex api key to be masked")
		}
	}

	// Delete clears active selection.
	if err := s.Delete(p1.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := s.Active().Claude; got != "" {
		t.Fatalf("expected active claude to be cleared, got %q", got)
	}

	// Persistence across reload.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore(reload): %v", err)
	}
	if got := len(s2.Profiles()); got != 1 {
		t.Fatalf("expected 1 profile after reload, got %d", got)
	}

	// File permissions (best-effort; Windows does not preserve chmod bits).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "providers.json"))
		if err != nil {
			t.Fatalf("stat providers.json: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("providers.json perm got=%#o want=%#o", got, 0o600)
		}
	}
}

func TestStoreUpsert_RequiresName(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s.Upsert(Profile{Name: " "}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStoreReorder_ValidatesInput(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p1, err := s.Upsert(Profile{Name: "A"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	p2, err := s.Upsert(Profile{Name: "B"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := s.Reorder([]string{p1.ID}); err == nil {
		t.Fatalf("expected length mismatch error")
	}
	if err := s.Reorder([]string{p1.ID, p1.ID}); err == nil {
		t.Fatalf("expected duplicate error")
	}
	if err := s.Reorder([]string{p1.ID, "missing"}); err == nil {
		t.Fatalf("expected unknown id error")
	}
	if err := s.Reorder([]string{p2.ID, p1.ID}); err != nil {
		t.Fatalf("expected ok reorder, got %v", err)
	}
}

func TestStoreUpsert_RejectsDuplicateName(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	first, err := s.Upsert(Profile{Name: "Current"})
	if err != nil {
		t.Fatalf("Upsert(first): %v", err)
	}
	if _, err := s.Upsert(Profile{Name: "current"}); err == nil {
		t.Fatalf("expected duplicate-name error")
	}

	other, err := s.Upsert(Profile{Name: "Other"})
	if err != nil {
		t.Fatalf("Upsert(other): %v", err)
	}
	other.Name = "CURRENT"
	if _, err := s.Upsert(other); err == nil {
		t.Fatalf("expected duplicate-name error on update")
	}

	// Same profile may keep its own name.
	first.Name = "  Current  "
	if _, err := s.Upsert(first); err != nil {
		t.Fatalf("upsert same profile name: %v", err)
	}
}
