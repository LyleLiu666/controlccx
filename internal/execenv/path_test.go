package execenv

import (
	"os"
	"strings"
	"testing"
)

func TestPrependPATH_MissingPATH(t *testing.T) {
	env := []string{"FOO=bar"}
	out, changed := PrependPATH(env, []string{"/x/bin", "/y/bin"})
	if !changed {
		t.Fatalf("expected changed")
	}
	got := envGet(out, "PATH")
	if got == "" {
		t.Fatalf("expected PATH to be set")
	}
	sep := string(os.PathListSeparator)
	if !strings.HasPrefix(got, "/x/bin"+sep+"/y/bin") {
		t.Fatalf("unexpected PATH prefix: %q", got)
	}
}

func TestPrependPATH_PreservesExistingAndAvoidsDuplicates(t *testing.T) {
	sep := string(os.PathListSeparator)
	env := []string{"PATH=/usr/bin" + sep + "/bin", "FOO=bar"}
	out, changed := PrependPATH(env, []string{"/usr/bin", "/x/bin", "/bin"})
	if !changed {
		t.Fatalf("expected changed")
	}
	got := envGet(out, "PATH")
	wantPrefix := "/x/bin" + sep + "/usr/bin" + sep + "/bin"
	if got != wantPrefix {
		t.Fatalf("got PATH=%q want %q", got, wantPrefix)
	}
}

func envGet(env []string, key string) string {
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			continue
		}
		return strings.TrimPrefix(kv, key+"=")
	}
	return ""
}
