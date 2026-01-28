package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxFSWriteBytes = 1 << 20 // 1 MiB

func (a *API) fsRootsOrDefault() []FSRoot {
	roots := a.FSRoots
	if len(roots) == 0 {
		roots = DefaultFSRoots()
	}
	return roots
}

func resolveFSPath(rawPath, baseRaw string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("path is required")
	}

	path := filepath.Clean(rawPath)
	if baseRaw != "" && !filepath.IsAbs(path) {
		base := filepath.Clean(strings.TrimSpace(baseRaw))
		if !filepath.IsAbs(base) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("cannot resolve cwd")
			}
			base = filepath.Join(cwd, base)
		}
		path = filepath.Join(base, path)
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot resolve cwd")
		}
		path = filepath.Join(cwd, path)
	}
	return path, nil
}

func (a *API) handleFSWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Path    string `json:"path"`
		Base    string `json:"base,omitempty"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	path, err := resolveFSPath(body.Path, body.Base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.fsRootsOrDefault()
	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	data := []byte(body.Content)
	if len(data) > maxFSWriteBytes {
		http.Error(w, "content too large", http.StatusBadRequest)
		return
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		http.Error(w, "fs: not a file", http.StatusBadRequest)
		return
	}

	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		http.Error(w, "fs: parent directory not found", http.StatusBadRequest)
		return
	}

	if err := writeFileAtomic(path, data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]any{
		"ok":    true,
		"path":  path,
		"bytes": len(data),
	})
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, fmt.Sprintf(".controlccx-tmp-%d", time.Now().UnixNano()))
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("fs: write temp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			return fmt.Errorf("fs: rename: %w", err)
		}
	}
	return nil
}

type FSEntryKind string

const (
	FSEntryDir  FSEntryKind = "dir"
	FSEntryFile FSEntryKind = "file"
)

type FSEntry struct {
	Name string      `json:"name"`
	Path string      `json:"path"`
	Kind FSEntryKind `json:"kind"`
	Size int64       `json:"size,omitempty"`
}

type FSEntriesResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent,omitempty"`
	Entries []FSEntry `json:"entries"`
}

func (a *API) handleFSEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	baseRaw := strings.TrimSpace(r.URL.Query().Get("base"))

	path, err := resolveFSPath(raw, baseRaw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.fsRootsOrDefault()
	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		http.Error(w, "fs: not a directory", http.StatusBadRequest)
		return
	}

	list, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, fmt.Errorf("fs: readdir: %w", err).Error(), http.StatusBadRequest)
		return
	}

	var entries []FSEntry
	for _, e := range list {
		name := e.Name()
		full := filepath.Join(path, name)

		kind := FSEntryFile
		if e.IsDir() {
			kind = FSEntryDir
		} else if e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				kind = FSEntryDir
			}
		}

		size := int64(0)
		if kind == FSEntryFile {
			if info, err := os.Stat(full); err == nil {
				size = info.Size()
			}
		}
		entries = append(entries, FSEntry{Name: name, Path: full, Kind: kind, Size: size})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == FSEntryDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	resp := FSEntriesResponse{Path: path, Entries: entries}
	parent := filepath.Dir(path)
	if parent != path {
		resp.Parent = parent
	}
	writeJSON(w, resp)
}

func (a *API) handleFSMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Path      string `json:"path"`
		Base      string `json:"base,omitempty"`
		Recursive *bool  `json:"recursive,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	path, err := resolveFSPath(body.Path, body.Base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.fsRootsOrDefault()
	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	recursive := true
	if body.Recursive != nil {
		recursive = *body.Recursive
	}

	if recursive {
		if err := os.MkdirAll(path, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := os.Mkdir(path, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true, "path": path})
}

func (a *API) handleFSDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Path      string `json:"path"`
		Base      string `json:"base,omitempty"`
		Recursive *bool  `json:"recursive,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	path, err := resolveFSPath(body.Path, body.Base)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	roots := a.fsRootsOrDefault()
	if !isUnderAnyRoot(path, roots) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	recursive := false
	if body.Recursive != nil {
		recursive = *body.Recursive
	}

	if info.IsDir() && recursive {
		if err := os.RemoveAll(path); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := os.Remove(path); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true, "path": path})
}
