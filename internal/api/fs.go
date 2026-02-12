package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"controlccx/internal/fssec"
)

type FSRoot = fssec.Root

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
	return fssec.DefaultRoots()
}

func FSRootsFromPaths(paths []string) []FSRoot {
	return fssec.RootsFromPaths(paths)
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
	return fssec.IsUnderAnyRoot(path, roots)
}
