package tooling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestService_DefaultsAndOverride(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{
		DataDir: dir,
		Defaults: []Tool{
			{ID: "claude-code", Driver: DriverClaudeCode, Command: "claude"},
			{ID: "codex", Driver: DriverCodex, Command: "codex"},
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got := svc.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 defaults, got %d", len(got))
	}
	if _, ok := svc.Resolve("claude-code"); !ok {
		t.Fatalf("expected resolve claude-code")
	}

	if err := svc.Upsert(Tool{ID: "claude-code", Driver: DriverClaudeCode, Command: "/x/claude"}); err != nil {
		t.Fatalf("Upsert override: %v", err)
	}
	t1, ok := svc.Resolve("claude-code")
	if !ok || t1.Command != "/x/claude" {
		t.Fatalf("expected override to apply, got ok=%v command=%q", ok, t1.Command)
	}

	// Delete override should fall back to default.
	if err := svc.Delete("claude-code"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	t2, ok := svc.Resolve("claude-code")
	if !ok || t2.Command != "claude" {
		t.Fatalf("expected fallback default, got ok=%v command=%q", ok, t2.Command)
	}
}

func TestService_PersistAndReload(t *testing.T) {
	dir := t.TempDir()
	svc1, err := NewService(Options{
		DataDir: dir,
		Defaults: []Tool{
			{ID: "claude-code", Driver: DriverClaudeCode, Command: "claude"},
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if err := svc1.Upsert(Tool{
		ID:      "claude-cn",
		Driver:  DriverClaudeCode,
		Command: "claude",
		Env:     map[string]string{"ANTHROPIC_BASE_URL": "https://example.invalid"},
		Args:    []string{"--foo"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	path := filepath.Join(dir, "tools.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected tools.json to exist: %v", err)
	}

	svc2, err := NewService(Options{
		DataDir: dir,
		Defaults: []Tool{
			{ID: "claude-code", Driver: DriverClaudeCode, Command: "claude"},
		},
	})
	if err != nil {
		t.Fatalf("NewService(reload): %v", err)
	}
	got, ok := svc2.Resolve("claude-cn")
	if !ok {
		t.Fatalf("expected tool to persist")
	}
	if got.Env["ANTHROPIC_BASE_URL"] != "https://example.invalid" {
		t.Fatalf("env not persisted: %#v", got.Env)
	}
	if len(got.Args) != 1 || got.Args[0] != "--foo" {
		t.Fatalf("args not persisted: %#v", got.Args)
	}
}

func TestService_Validate(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if err := svc.Upsert(Tool{ID: "bad id", Driver: DriverClaudeCode, Command: "x"}); err == nil {
		t.Fatalf("expected invalid id error")
	}
	if err := svc.Upsert(Tool{ID: "ok", Driver: "nope", Command: "x"}); err == nil {
		t.Fatalf("expected invalid driver error")
	}
	if err := svc.Upsert(Tool{ID: "ok2", Driver: DriverClaudeCode, Command: ""}); err == nil {
		t.Fatalf("expected missing command error")
	}
}
