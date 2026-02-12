package fssec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Root struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func DefaultRoots() []Root {
	var roots []Root

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots, Root{Name: "Home", Path: filepath.Clean(home)})
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		roots = append(roots, Root{Name: "Cwd", Path: filepath.Clean(cwd)})
	}

	if runtime.GOOS == "windows" {
		for _, drive := range listWindowsDrives() {
			roots = append(roots, Root{Name: drive, Path: drive})
		}
	} else {
		roots = append(roots, Root{Name: "Root", Path: string(os.PathSeparator)})
	}

	roots = dedupeRoots(roots)
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].Name == roots[j].Name {
			return roots[i].Path < roots[j].Path
		}
		return roots[i].Name < roots[j].Name
	})
	return roots
}

func RootsFromPaths(paths []string) []Root {
	out := make([]Root, 0, len(paths))
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
		}
		out = append(out, Root{Name: "Root", Path: filepath.Clean(p)})
	}
	return dedupeRoots(out)
}

func EffectiveRoots(paths []string) []Root {
	roots := RootsFromPaths(paths)
	if len(roots) > 0 {
		return roots
	}
	return DefaultRoots()
}

func ResolvePath(rawPath, baseRaw string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", errors.New("path is required")
	}

	path := filepath.Clean(rawPath)
	if baseRaw != "" && !filepath.IsAbs(path) {
		base := filepath.Clean(strings.TrimSpace(baseRaw))
		if !filepath.IsAbs(base) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", errors.New("cannot resolve cwd")
			}
			base = filepath.Join(cwd, base)
		}
		path = filepath.Join(base, path)
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", errors.New("cannot resolve cwd")
		}
		path = filepath.Join(cwd, path)
	}
	return path, nil
}

func IsUnderAnyRoot(path string, roots []Root) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	pathAbs = evalSymlinksBestEffort(pathAbs)

	for _, r := range roots {
		rootAbs, err := filepath.Abs(r.Path)
		if err != nil {
			continue
		}
		rootAbs = evalSymlinksBestEffort(rootAbs)
		if isWithin(rootAbs, pathAbs) {
			return true
		}
	}
	return false
}

func evalSymlinksBestEffort(pathAbs string) string {
	pathAbs = filepath.Clean(pathAbs)
	p := pathAbs
	suffix := ""
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			if suffix != "" {
				resolved = filepath.Join(resolved, suffix)
			}
			return filepath.Clean(resolved)
		}

		if os.IsNotExist(err) {
			parent := filepath.Dir(p)
			if parent == p {
				if suffix != "" {
					return filepath.Join(p, suffix)
				}
				return pathAbs
			}
			base := filepath.Base(p)
			if suffix == "" {
				suffix = base
			} else {
				suffix = filepath.Join(base, suffix)
			}
			p = parent
			continue
		}

		if errors.Is(err, os.ErrPermission) {
			if suffix != "" {
				return filepath.Join(p, suffix)
			}
			return pathAbs
		}
		if suffix != "" {
			return filepath.Join(p, suffix)
		}
		return pathAbs
	}
}

func isWithin(rootAbs, pathAbs string) bool {
	rootAbs = filepath.Clean(rootAbs)
	pathAbs = filepath.Clean(pathAbs)

	if runtime.GOOS == "windows" {
		rootAbs = strings.ToLower(rootAbs)
		pathAbs = strings.ToLower(pathAbs)
	}

	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func dedupeRoots(in []Root) []Root {
	seen := make(map[string]struct{}, len(in))
	out := make([]Root, 0, len(in))
	for _, r := range in {
		p := filepath.Clean(strings.TrimSpace(r.Path))
		if p == "" {
			continue
		}
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		name := strings.TrimSpace(r.Name)
		if name == "" {
			name = "Root"
		}
		out = append(out, Root{Name: name, Path: p})
	}
	return out
}

func listWindowsDrives() []string {
	out := make([]string, 0, 4)
	for c := byte('A'); c <= byte('Z'); c++ {
		drive := string([]byte{c, ':'}) + string(os.PathSeparator)
		if info, err := os.Stat(drive); err == nil && info.IsDir() {
			out = append(out, drive)
		}
	}
	return out
}
