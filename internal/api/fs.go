package api

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type FSRoot struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type FSListEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type FSListResponse struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent,omitempty"`
	Entries []FSListEntry `json:"entries"`
}

func DefaultFSRoots() []FSRoot {
	var roots []FSRoot

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, FSRoot{Name: "Home", Path: filepath.Clean(home)})
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		roots = append(roots, FSRoot{Name: "Cwd", Path: filepath.Clean(cwd)})
	}

	if runtime.GOOS == "windows" {
		for _, drive := range listWindowsDrives() {
			roots = append(roots, FSRoot{Name: drive, Path: drive})
		}
	} else {
		roots = append(roots, FSRoot{Name: "Root", Path: string(os.PathSeparator)})
	}

	roots = dedupeRoots(roots)
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	return roots
}

func ListDirs(path string) (FSListResponse, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return FSListResponse{}, fmt.Errorf("fs: stat: %w", err)
	}
	if !info.IsDir() {
		return FSListResponse{}, fmt.Errorf("fs: not a directory")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return FSListResponse{}, fmt.Errorf("fs: readdir: %w", err)
	}

	var dirs []FSListEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			dirs = append(dirs, FSListEntry{Name: name, Path: filepath.Join(path, name)})
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(path, name)); err == nil && info.IsDir() {
				dirs = append(dirs, FSListEntry{Name: name, Path: filepath.Join(path, name)})
			}
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })

	resp := FSListResponse{Path: path, Entries: dirs}
	parent := filepath.Dir(path)
	if parent != path {
		resp.Parent = parent
	}
	return resp, nil
}

func isUnderAnyRoot(path string, roots []FSRoot) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, r := range roots {
		rootAbs, err := filepath.Abs(r.Path)
		if err != nil {
			continue
		}
		if isWithin(rootAbs, pathAbs) {
			return true
		}
	}
	return false
}

func isWithin(rootAbs, pathAbs string) bool {
	rootAbs = filepath.Clean(rootAbs)
	pathAbs = filepath.Clean(pathAbs)

	// Windows file systems are case-insensitive in practice.
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

func dedupeRoots(in []FSRoot) []FSRoot {
	seen := make(map[string]struct{}, len(in))
	var out []FSRoot
	for _, r := range in {
		p := filepath.Clean(r.Path)
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, FSRoot{Name: r.Name, Path: p})
	}
	return out
}

func listWindowsDrives() []string {
	var out []string
	for c := byte('A'); c <= byte('Z'); c++ {
		drive := string([]byte{c, ':'}) + string(os.PathSeparator)
		if info, err := os.Stat(drive); err == nil && info.IsDir() {
			out = append(out, drive)
		}
	}
	return out
}

