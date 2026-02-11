package worker

import (
	"testing"

	"controlccx/internal/tasks"
)

func TestCodexSandboxInputForTask_UsesTierDefaultsWhenTierOnly(t *testing.T) {
	tests := []struct {
		name string
		tier tasks.NetworkTier
		want string
	}{
		{name: "off", tier: tasks.NetworkTierOff, want: "read-only"},
		{name: "web_readonly", tier: tasks.NetworkTierWebReadonly, want: "workspace-write"},
		{name: "exec_net", tier: tasks.NetworkTierExecNet, want: "danger-full-access"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codexSandboxInputForTask(tasks.Task{
				WorkerType:  tasks.WorkerCodex,
				NetworkTier: tt.tier,
			})
			if got != tt.want {
				t.Fatalf("codexSandboxInputForTask(%q)=%q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}

func TestCodexSandboxInputForTask_PreservesExplicitSandbox(t *testing.T) {
	got := codexSandboxInputForTask(tasks.Task{
		WorkerType:   tasks.WorkerCodex,
		NetworkTier:  tasks.NetworkTierWebReadonly,
		CodexSandbox: "read-only",
	})
	if got != "read-only" {
		t.Fatalf("sandbox=%q, want %q", got, "read-only")
	}
}

func TestCodexSearchEnabledForTask_UsesTierDefaultsWhenTierOnly(t *testing.T) {
	if !codexSearchEnabledForTask(tasks.Task{WorkerType: tasks.WorkerCodex, NetworkTier: tasks.NetworkTierWebReadonly}) {
		t.Fatalf("expected web_readonly to enable codex search by default")
	}
	if !codexSearchEnabledForTask(tasks.Task{WorkerType: tasks.WorkerCodex, NetworkTier: tasks.NetworkTierExecNet}) {
		t.Fatalf("expected exec_net to enable codex search by default")
	}
	if codexSearchEnabledForTask(tasks.Task{WorkerType: tasks.WorkerCodex, NetworkTier: tasks.NetworkTierOff}) {
		t.Fatalf("expected off to disable codex search")
	}
}

func TestCodexSearchEnabledForTask_OffTierOverridesExplicitSearch(t *testing.T) {
	if codexSearchEnabledForTask(tasks.Task{
		WorkerType:  tasks.WorkerCodex,
		NetworkTier: tasks.NetworkTierOff,
		CodexSearch: true,
	}) {
		t.Fatalf("expected off tier to keep search disabled")
	}
}

func TestCodexSearchEnabledForTask_PreservesLegacyFalseWithExplicitSandbox(t *testing.T) {
	if codexSearchEnabledForTask(tasks.Task{
		WorkerType:   tasks.WorkerCodex,
		NetworkTier:  tasks.NetworkTierWebReadonly,
		CodexSandbox: "read-only",
		CodexSearch:  false,
	}) {
		t.Fatalf("expected explicit read-only sandbox to preserve codex_search=false")
	}
}
