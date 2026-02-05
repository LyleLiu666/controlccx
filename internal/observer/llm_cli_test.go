package observer

import (
	"testing"

	"controlccx/internal/config"
)

func TestClaudeCLIBackend_BuildArgs_IncludesSystemPromptAndDisablesTools(t *testing.T) {
	cfg := config.Default()
	cfg.Paths.DataDir = t.TempDir()

	b := &ClaudeCLIBackend{cfg: cfg}

	args := b.buildArgs()

	tools := indexOf(args, "--tools")
	if tools < 0 || tools+1 >= len(args) {
		t.Fatalf("expected --tools flag, args=%v", args)
	}
	if args[tools+1] != "" {
		t.Fatalf("expected --tools \"\" (disable builtin tools), got %q", args[tools+1])
	}

	sys := indexOf(args, "--system-prompt")
	if sys < 0 || sys+1 >= len(args) {
		t.Fatalf("expected --system-prompt flag with value, args=%v", args)
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

