package worker

import (
	"strconv"
	"strings"
	"sync"
)

const succeededWithToolErrorsWarning = "run succeeded but tool errors were observed; see stderr logs"

type toolErrorState struct {
	mu   sync.Mutex
	seen bool
}

func (s *toolErrorState) mark() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.seen = true
	s.mu.Unlock()
}

func (s *toolErrorState) seenAny() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen
}

func mergeWarning(existing string, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return existing
	}
	if existing == "" {
		return extra
	}
	if strings.Contains(existing, extra) {
		return existing
	}
	return existing + "\n" + extra
}

func parseToolResultExitCode(content string) (int, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, false
	}
	first, _, _ := strings.Cut(content, "\n")
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, "Exit code") {
		return 0, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(first, "Exit code"))
	rest = strings.TrimLeft(rest, " :=\t")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return 0, false
	}
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

func summarizeToolResultContent(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	if content == "" || maxRunes <= 0 {
		return ""
	}

	// Drop a leading "Exit code ..." line when present since we render exit separately.
	if first, rest, ok := strings.Cut(content, "\n"); ok {
		if strings.HasPrefix(strings.TrimSpace(first), "Exit code") {
			content = strings.TrimSpace(rest)
		}
	}
	if content == "" {
		content = strings.TrimSpace(content)
	}

	content = compactWhitespace(content)
	if content == "" {
		return ""
	}

	if runeLen(content) <= maxRunes {
		return content
	}

	// Prefer keeping the tail (errors often show up at the end) while still retaining some head context.
	head := 160
	tail := maxRunes - head - len(" ... ")
	if tail < 80 {
		// Extremely small budgets: just hard truncate.
		return truncateRunes(content, maxRunes)
	}

	return truncateRunes(truncateHeadTailRunes(content, head, tail), maxRunes)
}

func compactWhitespace(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func truncateHeadTailRunes(s string, head int, tail int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	r := []rune(s)
	if len(r) <= head+tail+len(" ... ") {
		return s
	}
	if head == 0 && tail == 0 {
		return ""
	}
	if head == 0 {
		if tail >= len(r) {
			return s
		}
		return "..." + string(r[len(r)-tail:])
	}
	if tail == 0 {
		if head >= len(r) {
			return s
		}
		return string(r[:head]) + "..."
	}
	if head >= len(r) {
		return s
	}
	if tail >= len(r) {
		return s
	}
	return string(r[:head]) + " ... " + string(r[len(r)-tail:])
}

func runeLen(s string) int {
	return len([]rune(s))
}
