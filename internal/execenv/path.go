package execenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultExtraPathDirs returns a conservative list of common bin directories that
// are often missing when the server is launched from a GUI (no shell init), but
// where CLI tools like "claude" / "codex" are commonly installed.
func DefaultExtraPathDirs() []string {
	home, _ := os.UserHomeDir()

	var dirs []string
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
		dirs = append(dirs, filepath.Join(home, "bin"))
		dirs = append(dirs, filepath.Join(home, ".npm-global", "bin"))
	}

	switch runtime.GOOS {
	case "darwin":
		// Homebrew (Apple Silicon / Intel)
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
	case "linux":
		dirs = append(dirs, "/usr/local/bin", "/usr/bin")
	case "windows":
		// No-op; Windows uses different execution and wrapper handling.
	default:
		dirs = append(dirs, "/usr/local/bin", "/usr/bin")
	}

	// De-dupe while preserving order.
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		key := d
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

// PrependPATH prepends the provided directories to PATH in env, keeping order,
// and avoiding duplicates. Returns the updated env and whether it changed.
func PrependPATH(env []string, dirs []string) ([]string, bool) {
	if len(dirs) == 0 {
		return env, false
	}

	keyEq := "PATH="
	matchKey := func(k string) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(k, "PATH")
		}
		return k == "PATH"
	}

	pathIdx := -1
	var existing string
	for i, kv := range env {
		j := strings.IndexByte(kv, '=')
		if j <= 0 {
			continue
		}
		k := kv[:j]
		if matchKey(k) {
			pathIdx = i
			existing = kv[j+1:]
			break
		}
	}

	sep := string(os.PathListSeparator)
	existingParts := splitPath(existing)
	existingSet := make(map[string]bool, len(existingParts))
	for _, p := range existingParts {
		existingSet[pathKey(p)] = true
	}

	prefix := make([]string, 0, len(dirs))
	changed := false
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		k := pathKey(d)
		if existingSet[k] {
			continue
		}
		existingSet[k] = true
		prefix = append(prefix, d)
		changed = true
	}

	if !changed {
		return env, false
	}

	combinedParts := append(prefix, existingParts...)
	combined := strings.Join(combinedParts, sep)

	out := append([]string{}, env...)
	if pathIdx >= 0 {
		out[pathIdx] = keyEq + combined
	} else {
		out = append(out, keyEq+combined)
	}
	return out, true
}

func splitPath(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func pathKey(p string) string {
	p = strings.TrimSpace(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}
