package main

import (
	"strings"

	"controlccx/internal/providers"
)

func normalizeSecretaryBackend(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "auto":
		return "auto"
	case "simple-http":
		return "simple-http"
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	default:
		return "auto"
	}
}

// resolveSecretaryBackend converts a requested backend into the effective backend for a secretary request.
//
// If the caller explicitly requests a concrete backend (simple-http/claude/codex), that override wins.
// Otherwise (auto/empty/invalid), it falls back to the active provider profile for secretary, if configured.
func resolveSecretaryBackend(requested string, store *providers.Store) string {
	req := normalizeSecretaryBackend(requested)
	if req != "auto" {
		return req
	}
	if store == nil {
		return req
	}
	active := store.Active()
	id := strings.TrimSpace(active.Secretary)
	if id == "" {
		return req
	}
	p, ok := store.Get(id)
	if !ok {
		return req
	}
	b := normalizeSecretaryBackend(p.Targets.Secretary.Backend)
	if b != "auto" {
		return b
	}
	return req
}

