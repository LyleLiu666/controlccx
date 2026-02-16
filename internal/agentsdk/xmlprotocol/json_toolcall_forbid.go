package xmlprotocol

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var errJSONToolCallForbidden = errors.New("json tool call is forbidden; use xml <tool_data> protocol")

func isForbiddenJSONToolCall(content string) bool {
	for _, candidate := range extractJSONCandidates(content) {
		var payload any
		if err := json.Unmarshal([]byte(candidate), &payload); err != nil {
			continue
		}
		if containsToolCallShape(payload) {
			return true
		}
	}
	return false
}

func extractJSONCandidates(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	candidates := make([]string, 0, 8)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		candidates = append(candidates, trimmed)
	}

	candidates = append(candidates, extractBalancedJSONSegments(content, 32)...)

	// de-duplicate and sort to keep behavior deterministic for tests
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	sort.Strings(out)
	return out
}

func extractBalancedJSONSegments(text string, maxSegments int) []string {
	if strings.TrimSpace(text) == "" || maxSegments <= 0 {
		return nil
	}

	segments := make([]string, 0, 4)
	stack := make([]rune, 0, 16)
	start := -1
	inString := false
	escape := false

	reset := func() {
		stack = stack[:0]
		start = -1
		inString = false
		escape = false
	}

	for idx, r := range text {
		if start == -1 {
			if r == '{' || r == '[' {
				start = idx
				stack = append(stack[:0], r)
				inString = false
				escape = false
			}
			continue
		}

		if inString {
			if escape {
				escape = false
				continue
			}
			if r == '\\' {
				escape = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, r)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				reset()
				continue
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				reset()
				continue
			}
			stack = stack[:len(stack)-1]
		}

		if len(stack) == 0 {
			end := idx + len(string(r))
			segment := strings.TrimSpace(text[start:end])
			if segment != "" {
				segments = append(segments, segment)
				if len(segments) >= maxSegments {
					return segments
				}
			}
			reset()
		}
	}

	return segments
}

func containsToolCallShape(v any) bool {
	switch val := v.(type) {
	case map[string]any:
		for key, nested := range val {
			normalized := strings.ToLower(strings.TrimSpace(key))
			switch normalized {
			case "agent_action", "tool_name", "tool", "tool_call", "tool_calls", "calls", "tools":
				return true
			}
			if containsToolCallShape(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range val {
			if containsToolCallShape(nested) {
				return true
			}
		}
	}
	return false
}
