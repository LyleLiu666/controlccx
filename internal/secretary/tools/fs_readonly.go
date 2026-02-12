package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
	"controlccx/internal/fssec"
)

const (
	fsEntriesDefaultLimit = 200
	fsEntriesMaxLimit     = 500
	fsReadDefaultMaxBytes = 64 << 10
	fsReadMaxBytes        = 1 << 20
)

type fsRootsTool struct{}

func (fsRootsTool) Name() string { return "fs_roots" }

func (fsRootsTool) DescriptionZH() string {
	return "列出秘书可读取的文件系统根目录（只读）。无参数。"
}

func (fsRootsTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	_ = call
	return map[string]any{"roots": fssec.EffectiveRoots(deps.FSRoots)}, nil
}

type fsPWDTool struct{}

func (fsPWDTool) Name() string { return "fs_pwd" }

func (fsPWDTool) DescriptionZH() string {
	return "获取秘书当前目录（只读）。优先返回当前进程工作目录；若不在可读根内则回退到第一个可读根。无参数。"
}

func (fsPWDTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	_ = call
	roots := fssec.EffectiveRoots(deps.FSRoots)
	path, source, err := fsPWDPath(roots)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":   path,
		"source": source,
	}, nil
}

type fsEntriesTool struct{}

func (fsEntriesTool) Name() string { return "fs_entries" }

func (fsEntriesTool) DescriptionZH() string {
	return "列出目录内容（只读）。参数：path（必填）、base（可选）、include_hidden（可选，默认false）、kind（可选：all/dir/file，默认all）、limit（可选，默认200，最大500）。"
}

func (fsEntriesTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	path, err := fssec.ResolvePath(call.Fields["path"], call.Fields["base"])
	if err != nil {
		return nil, err
	}
	roots := fssec.EffectiveRoots(deps.FSRoots)
	if !fssec.IsUnderAnyRoot(path, roots) {
		return nil, errors.New("path not allowed")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("fs: not a directory")
	}

	includeHidden := parseBool(call.Fields["include_hidden"])
	kindFilter := strings.ToLower(strings.TrimSpace(call.Fields["kind"]))
	switch kindFilter {
	case "", "all", "dir", "file":
	default:
		return nil, fmt.Errorf("unknown kind %q (allowed: all, dir, file)", kindFilter)
	}
	if kindFilter == "" {
		kindFilter = "all"
	}

	limit := parseInt(call.Fields["limit"], fsEntriesDefaultLimit)
	if limit <= 0 {
		limit = fsEntriesDefaultLimit
	}
	if limit > fsEntriesMaxLimit {
		limit = fsEntriesMaxLimit
	}

	list, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("fs: readdir: %w", err)
	}

	entries := make([]fsEntry, 0, len(list))
	for _, e := range list {
		name := e.Name()
		if !includeHidden && strings.HasPrefix(name, ".") {
			continue
		}

		full := filepath.Join(path, name)
		kind := fsEntryFile
		if e.IsDir() {
			kind = fsEntryDir
		} else if e.Type()&os.ModeSymlink != 0 {
			if stat, err := os.Stat(full); err == nil && stat.IsDir() {
				kind = fsEntryDir
			}
		}

		if kindFilter == "dir" && kind != fsEntryDir {
			continue
		}
		if kindFilter == "file" && kind != fsEntryFile {
			continue
		}

		size := int64(0)
		if kind == fsEntryFile {
			if stat, err := os.Stat(full); err == nil {
				size = stat.Size()
			}
		}
		entries = append(entries, fsEntry{
			Name: name,
			Path: full,
			Kind: kind,
			Size: size,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == fsEntryDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}

	resp := map[string]any{
		"path":           path,
		"entries":        entries,
		"count":          len(entries),
		"limit":          limit,
		"include_hidden": includeHidden,
		"kind":           kindFilter,
	}
	parent := filepath.Dir(path)
	if parent != path && fssec.IsUnderAnyRoot(parent, roots) {
		resp["parent"] = parent
	}
	return resp, nil
}

type fsReadTextTool struct{}

func (fsReadTextTool) Name() string { return "fs_read_text" }

func (fsReadTextTool) DescriptionZH() string {
	return "读取文本文件内容（只读）。参数：path（必填）、base（可选）、max_bytes（可选，默认65536，最大1048576）。"
}

func (fsReadTextTool) Execute(ctx context.Context, call agentsdk.ToolCall, deps Deps) (any, error) {
	_ = ctx
	path, err := fssec.ResolvePath(call.Fields["path"], call.Fields["base"])
	if err != nil {
		return nil, err
	}
	roots := fssec.EffectiveRoots(deps.FSRoots)
	if !fssec.IsUnderAnyRoot(path, roots) {
		return nil, errors.New("path not allowed")
	}

	maxBytes := parseInt(call.Fields["max_bytes"], fsReadDefaultMaxBytes)
	if maxBytes <= 0 {
		maxBytes = fsReadDefaultMaxBytes
	}
	if maxBytes > fsReadMaxBytes {
		maxBytes = fsReadMaxBytes
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("fs: not a file")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("fs: not a regular file")
	}

	data, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	truncated := false
	if len(data) > maxBytes {
		truncated = true
		data = data[:maxBytes]
	}

	return map[string]any{
		"path":       path,
		"size":       info.Size(),
		"truncated":  truncated,
		"max_bytes":  maxBytes,
		"utf8_valid": utf8.Valid(data),
		"content":    string(data),
	}, nil
}

type fsEntryKind string

const (
	fsEntryDir  fsEntryKind = "dir"
	fsEntryFile fsEntryKind = "file"
)

type fsEntry struct {
	Name string      `json:"name"`
	Path string      `json:"path"`
	Kind fsEntryKind `json:"kind"`
	Size int64       `json:"size,omitempty"`
}

func fsPWDPath(roots []fssec.Root) (path string, source string, err error) {
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil && strings.TrimSpace(cwd) != "" {
		if resolved, resolveErr := fssec.ResolvePath(cwd, ""); resolveErr == nil && fssec.IsUnderAnyRoot(resolved, roots) {
			return resolved, "cwd", nil
		}
	}
	if len(roots) == 0 {
		return "", "", errors.New("no readable roots available")
	}
	return filepath.Clean(roots[0].Path), "root_fallback", nil
}
