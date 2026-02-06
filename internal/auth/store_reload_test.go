package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_Reload_PicksUpExternalChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	if err := os.WriteFile(path, []byte(`{"anthropic_auth_token":"t1"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write initial: %v", err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := store.Get().AnthropicAuthToken; got != "t1" {
		t.Fatalf("initial token=%q want %q", got, "t1")
	}

	if err := os.WriteFile(path, []byte(`{"anthropic_auth_token":"t2"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write updated: %v", err)
	}

	if err := store.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := store.Get().AnthropicAuthToken; got != "t2" {
		t.Fatalf("reloaded token=%q want %q", got, "t2")
	}
}
