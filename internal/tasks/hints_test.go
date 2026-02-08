package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedTestCommands(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module example.com/x\n")
	mustWrite(t, filepath.Join(dir, "package.json"), `{"name":"x"}`)
	mustWrite(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 9\n")
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o755); err != nil {
		t.Fatalf("mkdir web: %v", err)
	}
	mustWrite(t, filepath.Join(dir, "web", "package.json"), `{"name":"web"}`)

	cmds := SuggestedTestCommands(dir)
	want := map[string]bool{
		"go test ./...":     false,
		"pnpm build":        false,
		"pnpm smoke":        false,
		"pnpm -C web build": false,
	}
	for _, c := range cmds {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for cmd, ok := range want {
		if !ok {
			t.Fatalf("missing %q in %v", cmd, cmds)
		}
	}
}

func TestFinishReason(t *testing.T) {
	ec := 2
	tests := []struct {
		name string
		task Task
		want string
	}{
		{"running", Task{Status: StatusRunning}, ""},
		{"succeeded", Task{Status: StatusSucceeded}, "succeeded"},
		{"failed_exit_code", Task{Status: StatusFailed, ExitCode: &ec}, "failed: exit code 2"},
		{"blocked", Task{Status: StatusBlocked}, "blocked: approval required"},
		{"failed_error", Task{Status: StatusFailed, Error: "boom"}, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FinishReason(tt.task); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
