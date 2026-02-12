package tools

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func parseBool(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	return v == "1" || v == "true" || v == "yes" || v == "y"
}

func parseInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func truncateDisplay(s string, max int) string {
	return truncateRunes(s, max)
}

func truncateUTF8Safe(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if max <= 0 {
		if s == "" {
			return "", false
		}
		return "", true
	}
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	return truncateRunes(s, max), true
}

func truncateUTF8SafeHeadTail(s string, head int, tail int) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	if head+tail <= 0 {
		return "", true
	}

	n := utf8.RuneCountInString(s)
	if n <= head+tail {
		return s, false
	}

	r := []rune(s)
	if head == 0 {
		return string(r[len(r)-tail:]), true
	}
	if tail == 0 {
		return truncateRunes(s, head), true
	}
	if head > len(r) {
		head = len(r)
	}
	if tail > len(r)-head {
		tail = len(r) - head
	}
	if tail < 0 {
		tail = 0
	}

	out := make([]rune, 0, head+tail)
	out = append(out, r[:head]...)
	// Mark the head/tail cut boundary without increasing output size.
	out[len(out)-1] = '…'
	out = append(out, r[len(r)-tail:]...)
	return string(out), true
}

func parseStringSliceCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
