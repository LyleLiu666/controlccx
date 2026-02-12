package main

import "testing"

func TestNeedsRunnerdRestart(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		observed string
		want     bool
	}{
		{
			name:     "no expected stamp means no forced restart",
			expected: "",
			observed: "",
			want:     false,
		},
		{
			name:     "matching stamp does not restart",
			expected: "/bin/ccx|123|456",
			observed: "/bin/ccx|123|456",
			want:     false,
		},
		{
			name:     "missing observed stamp restarts",
			expected: "/bin/ccx|123|456",
			observed: "",
			want:     true,
		},
		{
			name:     "mismatched stamp restarts",
			expected: "/bin/ccx|123|456",
			observed: "/bin/ccx|123|789",
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := needsRunnerdRestart(tc.expected, tc.observed)
			if got != tc.want {
				t.Fatalf("needsRunnerdRestart(%q,%q)=%v want=%v", tc.expected, tc.observed, got, tc.want)
			}
		})
	}
}
