package tasks

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxProjectContextRunesForObserver bounds context injected into the Observer LLM prompt.
	MaxProjectContextRunesForObserver = 6000
	// MaxProjectContextRunesForWorker bounds context prefixed to worker stdin prompts (claude-code/codex).
	MaxProjectContextRunesForWorker = 6000
)

func NormalizeProjectContext(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blankStreak := 0
	for _, raw := range lines {
		line := strings.TrimRightFunc(raw, unicode.IsSpace)
		if strings.TrimSpace(line) == "" {
			if blankStreak >= 1 {
				continue
			}
			out = append(out, "")
			blankStreak++
			continue
		}
		blankStreak = 0
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func CompressProjectContext(s string, maxRunes int) (string, bool) {
	s = NormalizeProjectContext(s)
	if s == "" || maxRunes <= 0 {
		return s, false
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s, false
	}

	keep := maxRunes - 1
	if keep < 1 {
		return "…", true
	}
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if n >= keep {
			break
		}
		b.WriteRune(r)
		n++
	}
	b.WriteRune('…')
	return b.String(), true
}
