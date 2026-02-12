package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
)

func TestFSRoots_UsesConfiguredRoots(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	reg := NewRegistry(Deps{FSRoots: []string{root}})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{Name: "fs_roots"})
	if err != nil {
		t.Fatalf("fs_roots: %v", err)
	}

	var out struct {
		Roots []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"roots"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if len(out.Roots) != 1 {
		t.Fatalf("roots len=%d want 1", len(out.Roots))
	}
	if filepath.Clean(out.Roots[0].Path) != filepath.Clean(root) {
		t.Fatalf("root path=%q want %q", out.Roots[0].Path, root)
	}
}

func TestFSPwd_UsesCWDWhenAllowed(t *testing.T) {
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	reg := NewRegistry(Deps{FSRoots: []string{cwd}})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{Name: "fs_pwd"})
	if err != nil {
		t.Fatalf("fs_pwd: %v", err)
	}

	var out struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if filepath.Clean(out.Path) != filepath.Clean(cwd) {
		t.Fatalf("path=%q want %q", out.Path, cwd)
	}
	if out.Source != "cwd" {
		t.Fatalf("source=%q want cwd", out.Source)
	}
}

func TestFSPwd_FallsBackToFirstRootWhenCWDNotAllowed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	reg := NewRegistry(Deps{FSRoots: []string{root}})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{Name: "fs_pwd"})
	if err != nil {
		t.Fatalf("fs_pwd: %v", err)
	}

	var out struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if filepath.Clean(out.Path) != filepath.Clean(root) {
		t.Fatalf("path=%q want %q", out.Path, root)
	}
	if out.Source != "root_fallback" {
		t.Fatalf("source=%q want root_fallback", out.Source)
	}
}

func TestFSEntries_RespectsRootsAndHiddenFlag(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatalf("write beta.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("write .hidden: %v", err)
	}

	reg := NewRegistry(Deps{FSRoots: []string{root}})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_entries",
		Fields: map[string]string{
			"path": root,
		},
	})
	if err != nil {
		t.Fatalf("fs_entries: %v", err)
	}

	var out struct {
		Path    string `json:"path"`
		Entries []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"entries"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}

	if filepath.Clean(out.Path) != filepath.Clean(root) {
		t.Fatalf("path=%q want %q", out.Path, root)
	}
	if len(out.Entries) != 2 {
		t.Fatalf("entries len=%d want 2", len(out.Entries))
	}
	if out.Entries[0].Kind != "dir" {
		t.Fatalf("entries[0].kind=%q want dir", out.Entries[0].Kind)
	}
	for _, e := range out.Entries {
		if e.Name == ".hidden" {
			t.Fatalf("hidden entry should be excluded by default")
		}
	}

	outAny, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_entries",
		Fields: map[string]string{
			"path":           root,
			"include_hidden": "true",
		},
	})
	if err != nil {
		t.Fatalf("fs_entries include_hidden: %v", err)
	}
	raw, _ = json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output include_hidden: %v raw=%s", err, string(raw))
	}
	if len(out.Entries) != 3 {
		t.Fatalf("entries len=%d want 3 when include_hidden=true", len(out.Entries))
	}

	outside := t.TempDir()
	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_entries",
		Fields: map[string]string{
			"path": outside,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "path not allowed") {
		t.Fatalf("expected path not allowed error, got: %v", err)
	}
}

func TestFSReadText_TruncatesAndBlocksOutside(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "note.txt")
	content := strings.Repeat("abc", 300)
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write note.txt: %v", err)
	}

	reg := NewRegistry(Deps{FSRoots: []string{root}})

	outAny, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_read_text",
		Fields: map[string]string{
			"path":      file,
			"max_bytes": "128",
		},
	})
	if err != nil {
		t.Fatalf("fs_read_text: %v", err)
	}

	var out struct {
		Path      string `json:"path"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
		MaxBytes  int    `json:"max_bytes"`
		Content   string `json:"content"`
	}
	raw, _ := json.Marshal(outAny)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output: %v raw=%s", err, string(raw))
	}
	if filepath.Clean(out.Path) != filepath.Clean(file) {
		t.Fatalf("path=%q want %q", out.Path, file)
	}
	if out.Size != int64(len(content)) {
		t.Fatalf("size=%d want %d", out.Size, len(content))
	}
	if !out.Truncated {
		t.Fatalf("truncated=false want true")
	}
	if out.MaxBytes != 128 {
		t.Fatalf("max_bytes=%d want 128", out.MaxBytes)
	}
	if len(out.Content) != 128 {
		t.Fatalf("content len=%d want 128", len(out.Content))
	}

	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_read_text",
		Fields: map[string]string{
			"path": root,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("expected not a file error, got: %v", err)
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}
	_, err = reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_read_text",
		Fields: map[string]string{
			"path": secret,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "path not allowed") {
		t.Fatalf("expected path not allowed error, got: %v", err)
	}
}

func TestFSReadText_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior varies on windows permissions")
	}

	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reg := NewRegistry(Deps{FSRoots: []string{root}})
	_, err := reg.Execute(ctx, agentsdk.ToolCall{
		Name: "fs_read_text",
		Fields: map[string]string{
			"path": filepath.Join(link, "secret.txt"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "path not allowed") {
		t.Fatalf("expected path not allowed, got: %v", err)
	}
}
