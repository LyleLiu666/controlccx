package main

import (
	"os"
	"path/filepath"
	"testing"

	"controlccx/internal/providers"
)

func TestResolveSecretaryBackend_UsesActiveProviderOnAuto(t *testing.T) {
	dataDir := t.TempDir()
	store, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p, err := store.Upsert(providers.Profile{
		Name: "P1",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{Backend: "codex"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.SetActive("secretary", p.ID); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	got := resolveSecretaryBackend("auto", store)
	if got != "codex" {
		t.Fatalf("got=%q want=%q", got, "codex")
	}

	got = resolveSecretaryBackend("", store)
	if got != "codex" {
		t.Fatalf("got=%q want=%q", got, "codex")
	}
}

func TestResolveSecretaryBackend_ExplicitOverrideWins(t *testing.T) {
	dataDir := t.TempDir()
	store, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p, err := store.Upsert(providers.Profile{
		Name: "P1",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{Backend: "codex"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.SetActive("secretary", p.ID); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	got := resolveSecretaryBackend("claude", store)
	if got != "claude" {
		t.Fatalf("got=%q want=%q", got, "claude")
	}
}

func TestResolveSecretaryBackend_InvalidRequestedFallsBack(t *testing.T) {
	dataDir := t.TempDir()
	store, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	p, err := store.Upsert(providers.Profile{
		Name: "P1",
		Targets: providers.Targets{
			Secretary: providers.SecretaryTarget{Backend: "simple-http"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := store.SetActive("secretary", p.ID); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	got := resolveSecretaryBackend("not-a-real-backend", store)
	if got != "simple-http" {
		t.Fatalf("got=%q want=%q", got, "simple-http")
	}
}

func TestResolveSecretaryBackend_MissingProviderFallsBackToAuto(t *testing.T) {
	dataDir := t.TempDir()
	store, err := providers.NewStore(dataDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Active references a missing id (simulate broken file).
	if err := os.WriteFile(
		filepath.Join(dataDir, "providers.json"),
		[]byte(`{"profiles":[],"active":{"secretary":"missing"}}`+"\n"),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile providers.json: %v", err)
	}
	if err := store.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := resolveSecretaryBackend("auto", store)
	if got != "auto" {
		t.Fatalf("got=%q want=%q", got, "auto")
	}
}

