package daemon

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreateInstanceToken_CreatesAndReuses(t *testing.T) {
	dir := t.TempDir()

	t1, err := LoadOrCreateInstanceToken(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateInstanceToken: %v", err)
	}
	if strings.TrimSpace(t1) == "" {
		t.Fatalf("expected non-empty token")
	}

	t2, err := LoadOrCreateInstanceToken(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateInstanceToken(reuse): %v", err)
	}
	if t2 != t1 {
		t.Fatalf("expected stable token, got %q then %q", t1, t2)
	}

	path := filepath.Join(dir, "instance.token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected token file to exist: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("token path is a directory")
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("perm=%o want %o", got, 0o600)
		}
	}
}

func TestHasValidInstanceToken_ValidatesHeader(t *testing.T) {
	h := http.Header{}
	h.Set(InstanceTokenHeader, "t1")
	if !HasValidInstanceToken(h, "t1") {
		t.Fatalf("expected valid")
	}
	if HasValidInstanceToken(h, "t2") {
		t.Fatalf("expected invalid token")
	}
	if HasValidInstanceToken(http.Header{}, "t1") {
		t.Fatalf("expected missing header to be invalid")
	}
	if HasValidInstanceToken(h, "") {
		t.Fatalf("expected missing expected token to be invalid")
	}
}
