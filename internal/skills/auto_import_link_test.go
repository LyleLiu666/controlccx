package skills

import (
	"context"
	"path/filepath"
	"testing"
)

func TestService_LinkWithAutoImport_PrefersToolsInOrder(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	ctx := context.Background()

	type tc struct {
		name         string
		present      []Target
		enableTarget Target
		want         Target
	}
	cases := []tc{
		{
			name:         "prefers claude_code when present",
			present:      []Target{TargetCursor, TargetCodex, TargetClaudeCode},
			enableTarget: TargetAntigravity,
			want:         TargetClaudeCode,
		},
		{
			name:         "falls back to codex when claude_code missing",
			present:      []Target{TargetCursor, TargetCodex},
			enableTarget: TargetAntigravity,
			want:         TargetCodex,
		},
		{
			name:         "falls back to antigravity before opencode",
			present:      []Target{TargetCursor, TargetOpencode, TargetAntigravity},
			enableTarget: TargetCodex,
			want:         TargetAntigravity,
		},
		{
			name:         "falls back to opencode before cursor",
			present:      []Target{TargetCursor, TargetOpencode},
			enableTarget: TargetCodex,
			want:         TargetOpencode,
		},
		{
			name:         "cursor is last resort",
			present:      []Target{TargetCursor},
			enableTarget: TargetCodex,
			want:         TargetCursor,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := filepath.Join(home, ".agent", "skills")

			svc, err := NewService(Options{HomeDir: home, SourceRoots: []string{sourceRoot}})
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			for _, tool := range c.present {
				for _, root := range svc.targetRoots[tool] {
					dir := filepath.Join(root, "skill-x")
					mustMkdir(t, dir)
					mustWrite(t, filepath.Join(dir, "SKILL.md"), "same\n")
				}
			}

			if err := svc.LinkWithAutoImport(ctx, "skill-x", c.enableTarget, AutoImportLinkOptions{}); err != nil {
				t.Fatalf("link with auto-import: %v", err)
			}

			m, err := readManagedManifest(filepath.Join(sourceRoot, "skill-x"))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			if got := Target(m.SourceTool); got != c.want {
				t.Fatalf("source_tool=%q, want %q", m.SourceTool, c.want)
			}
		})
	}
}
