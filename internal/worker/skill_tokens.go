package worker

import (
	"os"
	"path/filepath"
	"strings"

	"controlccx/internal/tasks"
)

func normalizePromptSkillTokensForExecution(worker tasks.WorkerType, prompt string) (string, int) {
	known := listKnownSkillNamesForWorker(worker)
	return normalizeSkillTokensForExecution(worker, prompt, known)
}

func normalizeSkillTokensForExecution(worker tasks.WorkerType, prompt string, knownSkillNames map[string]bool) (string, int) {
	switch worker {
	case tasks.WorkerCodex:
		return rewriteSkillTokens(prompt, '/', '$', knownSkillNames)
	case tasks.WorkerClaudeCode:
		return rewriteSkillTokens(prompt, '$', '/', knownSkillNames)
	default:
		return prompt, 0
	}
}

func rewriteSkillTokens(prompt string, fromPrefix, toPrefix byte, knownSkillNames map[string]bool) (string, int) {
	if prompt == "" || fromPrefix == toPrefix {
		return prompt, 0
	}
	if len(knownSkillNames) == 0 {
		return prompt, 0
	}

	var b strings.Builder
	b.Grow(len(prompt))

	changes := 0
	for i := 0; i < len(prompt); i++ {
		ch := prompt[i]
		if ch != fromPrefix {
			b.WriteByte(ch)
			continue
		}

		if i > 0 && !isWhitespace(prompt[i-1]) {
			b.WriteByte(ch)
			continue
		}

		nameStart := i + 1
		if nameStart >= len(prompt) || !isSkillNameStart(prompt[nameStart]) {
			b.WriteByte(ch)
			continue
		}

		nameEnd := nameStart
		for nameEnd < len(prompt) && isSkillNameChar(prompt[nameEnd]) {
			nameEnd++
		}
		name := prompt[nameStart:nameEnd]
		if strings.Contains(name, "..") {
			b.WriteByte(ch)
			continue
		}

		if nameEnd < len(prompt) && !isWhitespace(prompt[nameEnd]) {
			b.WriteByte(ch)
			continue
		}

		if !knownSkillNames[name] {
			b.WriteByte(ch)
			continue
		}

		changes++
		b.WriteByte(toPrefix)
		b.WriteString(name)
		i = nameEnd - 1
	}

	if changes == 0 {
		return prompt, 0
	}
	return b.String(), changes
}

func listKnownSkillNamesForWorker(worker tasks.WorkerType) map[string]bool {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	home = filepath.Clean(home)

	roots := []string{}
	switch worker {
	case tasks.WorkerCodex:
		roots = append(roots, filepath.Join(home, ".codex", "skills"))
		if raw := strings.TrimSpace(os.Getenv("CODEX_HOME")); raw != "" {
			ch := expandHomePath(raw, home)
			if !filepath.IsAbs(ch) {
				ch = filepath.Join(home, ch)
			}
			roots = append(roots, filepath.Join(filepath.Clean(ch), "skills"))
		}
	case tasks.WorkerClaudeCode:
		roots = append(roots, filepath.Join(home, ".claude", "skills"))
	}

	out := make(map[string]bool)
	for _, r := range dedupeCleanPaths(roots) {
		scanSkillNames(r, out)
	}
	return out
}

func scanSkillNames(root string, out map[string]bool) {
	entries, err := os.ReadDir(filepath.Clean(root))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSpace(e.Name())
		if !isSafeSkillName(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.IsDir() || (info.Mode()&os.ModeSymlink != 0) {
			out[name] = true
		}
	}
}

func isSafeSkillName(name string) bool {
	if name == "" {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return true
}

func isWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func isSkillNameStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

func isSkillNameChar(ch byte) bool {
	if isSkillNameStart(ch) {
		return true
	}
	switch ch {
	case '-', '_', '.', '@':
		return true
	default:
		return false
	}
}

func expandHomePath(p, home string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

func dedupeCleanPaths(paths []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		c := filepath.Clean(strings.TrimSpace(p))
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}
