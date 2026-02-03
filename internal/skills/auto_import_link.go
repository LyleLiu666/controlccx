package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AutoImportLinkOptions struct {
	PreferTool Target
}

type autoImportCandidate struct {
	Tool        Target
	Root        string
	EntryPath   string
	RealPath    string
	Fingerprint string
}

func autoImportToolRank(t Target) int {
	switch t {
	case TargetClaudeCode:
		return 0
	case TargetCodex:
		return 1
	case TargetAntigravity:
		return 2
	case TargetOpencode:
		return 3
	case TargetCursor:
		return 4
	default:
		return 100
	}
}

func (s *Service) LinkWithAutoImport(ctx context.Context, name string, target Target, opts AutoImportLinkOptions) error {
	if s == nil {
		return fmt.Errorf("skills: service is nil")
	}
	name = strings.TrimSpace(name)
	if !isSafeName(name) {
		return fmt.Errorf("skills: invalid skill name")
	}
	if _, ok := s.targetRoots[target]; !ok {
		return fmt.Errorf("skills: unknown target %q", target)
	}

	// Fast path: already managed.
	if _, err := s.resolveSourcePath(name); err == nil {
		return s.Link(ctx, name, target)
	}

	cand, err := s.pickAutoImportCandidate(name, opts.PreferTool)
	if err != nil {
		return err
	}
	if _, err := s.ImportExisting(ctx, ImportExistingInput{
		SourcePath: cand.RealPath,
		Name:       name,
		Tool:       string(cand.Tool),
		Overwrite:  false,
	}); err != nil {
		return err
	}
	return s.Link(ctx, name, target)
}

func (s *Service) pickAutoImportCandidate(name string, prefer Target) (autoImportCandidate, error) {
	cands, err := s.listAutoImportCandidates(name)
	if err != nil {
		return autoImportCandidate{}, err
	}
	if len(cands) == 0 {
		return autoImportCandidate{}, fmt.Errorf("skills: no importable variants found: %s", name)
	}

	sort.Slice(cands, func(i, j int) bool {
		ri := autoImportToolRank(cands[i].Tool)
		rj := autoImportToolRank(cands[j].Tool)
		if ri != rj {
			return ri < rj
		}
		if cands[i].Tool != cands[j].Tool {
			return cands[i].Tool < cands[j].Tool
		}
		if cands[i].Root != cands[j].Root {
			return cands[i].Root < cands[j].Root
		}
		return cands[i].EntryPath < cands[j].EntryPath
	})

	byFP := make(map[string][]autoImportCandidate)
	var fps []string
	for _, c := range cands {
		if _, ok := byFP[c.Fingerprint]; !ok {
			fps = append(fps, c.Fingerprint)
		}
		byFP[c.Fingerprint] = append(byFP[c.Fingerprint], c)
	}
	if len(fps) > 1 {
		sort.Strings(fps)
		lines := make([]string, 0, len(cands)+2)
		lines = append(lines, fmt.Sprintf(
			"MULTI_VARIANTS|skill %q has multiple different variants; import manually via governance:",
			name,
		))
		for _, c := range cands {
			lines = append(lines, fmt.Sprintf("- %s: %s", c.Tool, c.EntryPath))
		}
		return autoImportCandidate{}, fmt.Errorf("%s", strings.Join(lines, "\n"))
	}

	// Single fingerprint: pick deterministically, honoring preferred tool when possible.
	if strings.TrimSpace(string(prefer)) != "" {
		for _, c := range cands {
			if c.Tool == prefer {
				return c, nil
			}
		}
	}
	return cands[0], nil
}

func (s *Service) listAutoImportCandidates(name string) ([]autoImportCandidate, error) {
	if s == nil {
		return nil, fmt.Errorf("skills: service is nil")
	}

	var out []autoImportCandidate
	toolOrder := []Target{TargetClaudeCode, TargetCodex, TargetAntigravity, TargetOpencode, TargetCursor}
	for _, tool := range toolOrder {
		roots := s.targetRoots[tool]
		for _, root := range roots {
			entry := filepath.Join(root, name)
			info, err := os.Lstat(entry)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("skills: stat candidate: %w", err)
			}

			real := entry
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, err := filepath.EvalSymlinks(entry)
				if err != nil {
					continue
				}
				st, err := os.Stat(resolved)
				if err != nil || !st.IsDir() {
					continue
				}
				real = resolved
			} else if !info.IsDir() {
				continue
			}

			fp, err := dirFingerprint(real)
			if err != nil {
				return nil, err
			}
			out = append(out, autoImportCandidate{
				Tool:        tool,
				Root:        root,
				EntryPath:   entry,
				RealPath:    real,
				Fingerprint: fp,
			})
		}
	}
	return out, nil
}
