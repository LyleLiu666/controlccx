package tooling

import (
	"os"
	"path/filepath"
	"testing"
)

func TestService_Reload_PicksUpExternalToolChanges(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(Options{DataDir: dir})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	// Simulate an external write (another process updating tools.json).
	path := filepath.Join(dir, "tools.json")
	if err := os.WriteFile(path, []byte(`{"tools":[{"id":"t1","driver":"exec","command":"echo","args":["hi"]}]}`+"\n"), 0o600); err != nil {
		t.Fatalf("write tools.json: %v", err)
	}

	if _, ok := svc.Resolve("t1"); ok {
		t.Fatalf("resolve before reload: expected missing tool")
	}

	if err := svc.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	got, ok := svc.Resolve("t1")
	if !ok {
		t.Fatalf("resolve after reload: expected tool")
	}
	if got.Command != "echo" {
		t.Fatalf("command=%q want %q", got.Command, "echo")
	}
	if len(got.Args) != 1 || got.Args[0] != "hi" {
		t.Fatalf("args=%v want %v", got.Args, []string{"hi"})
	}
}
