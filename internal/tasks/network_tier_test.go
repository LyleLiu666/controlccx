package tasks

import "testing"

func TestNormalizeNetworkTier(t *testing.T) {
	tests := []struct {
		in   string
		want NetworkTier
	}{
		{in: "off", want: NetworkTierOff},
		{in: "no-network", want: NetworkTierOff},
		{in: "web_readonly", want: NetworkTierWebReadonly},
		{in: "search-browse", want: NetworkTierWebReadonly},
		{in: "exec_net", want: NetworkTierExecNet},
		{in: "unsafe", want: NetworkTierExecNet},
		{in: "invalid", want: ""},
	}
	for _, tt := range tests {
		if got := NormalizeNetworkTier(tt.in); got != tt.want {
			t.Fatalf("NormalizeNetworkTier(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveCreateNetworkTier(t *testing.T) {
	tier, err := resolveCreateNetworkTier(CreateTaskInput{
		NetworkTier: NetworkTierExecNet,
	})
	if err != nil {
		t.Fatalf("resolve explicit network tier: %v", err)
	}
	if tier != NetworkTierExecNet {
		t.Fatalf("tier=%q, want %q", tier, NetworkTierExecNet)
	}

	tier, err = resolveCreateNetworkTier(CreateTaskInput{
		SafetyPreset: "no-network",
	})
	if err != nil {
		t.Fatalf("resolve derived no-network: %v", err)
	}
	if tier != NetworkTierOff {
		t.Fatalf("tier=%q, want %q", tier, NetworkTierOff)
	}

	tier, err = resolveCreateNetworkTier(CreateTaskInput{
		ClaudeSandbox: false,
	})
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if tier != NetworkTierWebReadonly {
		t.Fatalf("tier=%q, want %q", tier, NetworkTierWebReadonly)
	}

	_, err = resolveCreateNetworkTier(CreateTaskInput{
		NetworkTier: NetworkTier("bad"),
	})
	if err == nil {
		t.Fatalf("expected invalid network tier error")
	}
}
