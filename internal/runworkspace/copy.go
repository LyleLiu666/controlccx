package runworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"controlccx/internal/tasks"
)

type copyManifest struct {
	Files map[string]string `json:"files"`
}

const maxHashBytes = 1 << 20 // 1 MiB

var excludedCopyDirsAnyDepth = map[string]bool{
	".ccx":         true,
	".git":         true,
	".venv":        true,
	"node_modules": true,
}

var excludedCopyDirsTopLevel = map[string]bool{
	"dist":  true,
	"build": true,
}

func isExcludedCopyPath(relSlash string) bool {
	relSlash = strings.TrimSpace(relSlash)
	if relSlash == "" || relSlash == "." {
		return false
	}
	relSlash = strings.Trim(relSlash, "/")
	if relSlash == "" {
		return false
	}
	parts := strings.Split(relSlash, "/")
	for _, p := range parts {
		if excludedCopyDirsAnyDepth[p] {
			return true
		}
	}
	if len(parts) > 0 && excludedCopyDirsTopLevel[parts[0]] {
		return true
	}
	return false
}

func createCopyWorkspace(baseWorkDir, runRoot string) (tasks.SessionWorkspace, error) {
	baseWorkDir = filepath.Clean(strings.TrimSpace(baseWorkDir))
	if baseWorkDir == "" {
		baseWorkDir = "."
	}
	runRoot = filepath.Clean(strings.TrimSpace(runRoot))
	if runRoot == "" {
		return tasks.SessionWorkspace{}, errors.New("runworkspace: run_root is required")
	}

	if err := os.MkdirAll(filepath.Dir(runRoot), 0o755); err != nil {
		return tasks.SessionWorkspace{}, fmt.Errorf("runworkspace: mkdir workspace parents: %w", err)
	}
	// Ensure a clean slate in case a previous run was partially created.
	_ = os.RemoveAll(runRoot)
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return tasks.SessionWorkspace{}, fmt.Errorf("runworkspace: mkdir workspace root: %w", err)
	}

	manifest, err := copyDirWithManifest(baseWorkDir, runRoot)
	if err != nil {
		return tasks.SessionWorkspace{}, err
	}
	if err := writeManifest(filepath.Join(runRoot, "manifest.json"), manifest); err != nil {
		return tasks.SessionWorkspace{}, err
	}

	return tasks.SessionWorkspace{
		Kind:        "copy",
		BaseWorkDir: baseWorkDir,
		RepoRoot:    "",
		RunRoot:     runRoot,
		RunWorkDir:  runRoot,
		Status:      "active",
	}, nil
}

func writeManifest(path string, m copyManifest) error {
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("runworkspace: marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("runworkspace: write manifest: %w", err)
	}
	return nil
}

func readManifest(path string) (copyManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return copyManifest{}, fmt.Errorf("runworkspace: read manifest: %w", err)
	}
	var m copyManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return copyManifest{}, fmt.Errorf("runworkspace: parse manifest: %w", err)
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m, nil
}

func applyBackCopyWorkspace(ws tasks.SessionWorkspace) ([]string, []string, error) {
	base := filepath.Clean(strings.TrimSpace(ws.BaseWorkDir))
	if base == "" {
		base = "."
	}
	runRoot := filepath.Clean(strings.TrimSpace(ws.RunRoot))
	if runRoot == "" {
		return nil, nil, errors.New("runworkspace: run_root is required")
	}

	manifestPath := filepath.Join(runRoot, "manifest.json")
	m, err := readManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	applied := []string(nil)
	conflicts := []string(nil)

	inManifest := map[string]bool{}
	for relSlash := range m.Files {
		inManifest[relSlash] = true
	}

	// Apply changes for manifest-managed files (snapshot-based conflict detection).
	for relSlash, snap := range m.Files {
		relOS := filepath.FromSlash(relSlash)
		basePath := filepath.Join(base, relOS)
		wsPath := filepath.Join(runRoot, relOS)

		baseFP, baseOK, err := fingerprintMaybe(basePath)
		if err != nil {
			return nil, nil, err
		}
		wsFP, wsOK, err := fingerprintMaybe(wsPath)
		if err != nil {
			return nil, nil, err
		}

		workspaceChanged := wsFP != snap
		if !workspaceChanged {
			continue
		}
		baseChanged := baseFP != snap
		if baseChanged && baseFP != wsFP {
			conflicts = append(conflicts, relSlash)
			continue
		}

		if wsOK {
			if err := copyPath(wsPath, basePath); err != nil {
				return nil, nil, err
			}
		} else if baseOK {
			_ = os.Remove(basePath)
		}
		applied = append(applied, relSlash)
	}

	// Apply new files created inside the workspace (best-effort).
	err = filepath.WalkDir(runRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == runRoot {
			return nil
		}
		rel, err := filepath.Rel(runRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.Clean(rel)
		relSlash := filepath.ToSlash(rel)
		if relSlash == "manifest.json" {
			return nil
		}
		if inManifest[relSlash] {
			if d.IsDir() {
				return nil
			}
			return nil
		}

		if isExcludedCopyPath(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		basePath := filepath.Join(base, filepath.FromSlash(relSlash))
		wsPath := path

		wsFP, _, err := fingerprintMaybe(wsPath)
		if err != nil {
			return err
		}
		baseFP, baseOK, err := fingerprintMaybe(basePath)
		if err != nil {
			return err
		}
		if baseOK && baseFP != wsFP {
			conflicts = append(conflicts, relSlash)
			return nil
		}
		if !baseOK {
			if err := copyPath(wsPath, basePath); err != nil {
				return err
			}
			applied = append(applied, relSlash)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("runworkspace: scan workspace files: %w", err)
	}

	sort.Strings(applied)
	sort.Strings(conflicts)
	return applied, conflicts, nil
}

func copyDirWithManifest(srcRoot, dstRoot string) (copyManifest, error) {
	manifest := copyManifest{Files: map[string]string{}}

	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcRoot {
			return nil
		}

		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.Clean(rel)
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." || relSlash == "" {
			return nil
		}

		if isExcludedCopyPath(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		dst := filepath.Join(dstRoot, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		mode := info.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(target, dst); err != nil {
				return err
			}
			fp := fingerprintSymlink(target)
			manifest.Files[relSlash] = fp
			return nil
		case d.IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			return nil
		case mode.IsRegular():
			if err := copyFile(path, dst, mode.Perm()); err != nil {
				return err
			}
			fp, err := fingerprintFile(path, info)
			if err != nil {
				return err
			}
			manifest.Files[relSlash] = fp
			return nil
		default:
			// Skip other file types.
			return nil
		}
	})
	if err != nil {
		return copyManifest{}, fmt.Errorf("runworkspace: copy workspace: %w", err)
	}
	return manifest, nil
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("runworkspace: stat %s: %w", src, err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("runworkspace: readlink %s: %w", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("runworkspace: mkdir %s: %w", filepath.Dir(dst), err)
		}
		_ = os.RemoveAll(dst)
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("runworkspace: symlink %s: %w", dst, err)
		}
		return nil
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return fmt.Errorf("runworkspace: mkdir %s: %w", dst, err)
		}
		return nil
	}
	if !mode.IsRegular() {
		return nil
	}
	return copyFile(src, dst, mode.Perm())
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("runworkspace: mkdir %s: %w", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("runworkspace: open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("runworkspace: create %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("runworkspace: copy %s: %w", dst, err)
	}
	return nil
}

func fingerprintSymlink(target string) string {
	sum := sha256.Sum256([]byte(target))
	return "symlink:" + hex.EncodeToString(sum[:])
}

func fingerprintFile(path string, info fs.FileInfo) (string, error) {
	if info.Size() > maxHashBytes {
		return fmt.Sprintf("meta:%d:%d", info.Size(), info.ModTime().UnixNano()), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("runworkspace: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("runworkspace: hash %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func fingerprintMaybe(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("runworkspace: stat %s: %w", path, err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", false, fmt.Errorf("runworkspace: readlink %s: %w", path, err)
		}
		return fingerprintSymlink(target), true, nil
	}
	if !mode.IsRegular() {
		return "", true, nil
	}
	fp, err := fingerprintFile(path, info)
	if err != nil {
		return "", false, err
	}
	return fp, true, nil
}
