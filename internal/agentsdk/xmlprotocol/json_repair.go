package xmlprotocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf16"
)

const maxJSONRepairBytes = 256 * 1024

// RepairJSON attempts to repair common JSON mistakes produced by LLMs.
//
// It is intentionally conservative: if the final output is not valid JSON,
// it returns ok=false and the caller should fall back to the original string.
func RepairJSON(input string) (repaired string, ok bool, steps []string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, false, nil
	}
	if len(trimmed) > maxJSONRepairBytes {
		return input, false, nil
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed, false, nil
	}

	out := trimmed
	changedSteps := make([]string, 0, 4)

	if next, changed := replacePythonLiteralsOutsideStrings(out); changed {
		out = next
		changedSteps = append(changedSteps, "python_literals")
	}
	if next, changed := convertSingleQuotedStrings(out); changed {
		out = next
		changedSteps = append(changedSteps, "single_quotes")
	}
	if next, changed := quoteUnquotedObjectKeys(out); changed {
		out = next
		changedSteps = append(changedSteps, "quote_keys")
	}
	if next, changed := removeTrailingCommas(out); changed {
		out = next
		changedSteps = append(changedSteps, "remove_trailing_commas")
	}
	if next, changed := closeUnclosedBrackets(out); changed {
		out = next
		changedSteps = append(changedSteps, "close_brackets")
	}

	out = strings.TrimSpace(out)
	if out == trimmed {
		return input, false, nil
	}
	if !json.Valid([]byte(out)) {
		return input, false, nil
	}
	return out, true, changedSteps
}

func removeTrailingCommas(input string) (string, bool) {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	quote := byte(0)
	escape := false
	changed := false

	for i := 0; i < len(input); i++ {
		c := input[i]

		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == quote {
				inString = false
				quote = 0
			}
			continue
		}

		if c == '"' || c == '\'' {
			inString = true
			quote = c
			b.WriteByte(c)
			continue
		}

		if c == ',' {
			j := i + 1
			for j < len(input) && isASCIISpace(input[j]) {
				j++
			}
			if j < len(input) && (input[j] == '}' || input[j] == ']') {
				changed = true
				continue
			}
		}

		b.WriteByte(c)
	}

	if !changed {
		return input, false
	}
	return b.String(), true
}

func replacePythonLiteralsOutsideStrings(input string) (string, bool) {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	quote := byte(0)
	escape := false
	changed := false

	for i := 0; i < len(input); {
		c := input[i]

		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
				i++
				continue
			}
			if c == '\\' {
				escape = true
				i++
				continue
			}
			if c == quote {
				inString = false
				quote = 0
			}
			i++
			continue
		}

		if c == '"' || c == '\'' {
			inString = true
			quote = c
			b.WriteByte(c)
			i++
			continue
		}

		if isIdentStart(c) {
			j := i + 1
			for j < len(input) && isIdentChar(input[j]) {
				j++
			}
			ident := input[i:j]
			switch ident {
			case "None":
				b.WriteString("null")
				changed = true
			case "True":
				b.WriteString("true")
				changed = true
			case "False":
				b.WriteString("false")
				changed = true
			default:
				b.WriteString(ident)
			}
			i = j
			continue
		}

		b.WriteByte(c)
		i++
	}

	if !changed {
		return input, false
	}
	return b.String(), true
}

func convertSingleQuotedStrings(input string) (string, bool) {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	quote := byte(0)
	escape := false
	changed := false

	for i := 0; i < len(input); {
		c := input[i]

		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
				i++
				continue
			}
			if c == '\\' {
				escape = true
				i++
				continue
			}
			if c == quote {
				inString = false
				quote = 0
			}
			i++
			continue
		}

		if c == '"' {
			inString = true
			quote = c
			b.WriteByte(c)
			i++
			continue
		}

		if c != '\'' {
			b.WriteByte(c)
			i++
			continue
		}

		// Parse a single-quoted string token (pseudo-JSON) and emit a proper JSON string.
		val, nextIdx, ok := parseSingleQuotedString(input, i)
		if !ok {
			// Leave as-is; other repair steps may still succeed.
			b.WriteByte(c)
			i++
			continue
		}
		encoded, _ := json.Marshal(val)
		b.Write(encoded)
		changed = true
		i = nextIdx
	}

	if !changed {
		return input, false
	}
	return b.String(), true
}

func parseSingleQuotedString(input string, start int) (value string, nextIdx int, ok bool) {
	if start < 0 || start >= len(input) || input[start] != '\'' {
		return "", start, false
	}

	var buf bytes.Buffer
	i := start + 1
	for i < len(input) {
		c := input[i]
		if c == '\'' {
			return buf.String(), i + 1, true
		}
		if c != '\\' {
			buf.WriteByte(c)
			i++
			continue
		}

		if i+1 >= len(input) {
			return "", start, false
		}
		esc := input[i+1]
		switch esc {
		case '\\':
			buf.WriteByte('\\')
			i += 2
		case '\'':
			buf.WriteByte('\'')
			i += 2
		case '"':
			buf.WriteByte('"')
			i += 2
		case 'n':
			buf.WriteByte('\n')
			i += 2
		case 'r':
			buf.WriteByte('\r')
			i += 2
		case 't':
			buf.WriteByte('\t')
			i += 2
		case 'b':
			buf.WriteByte('\b')
			i += 2
		case 'f':
			buf.WriteByte('\f')
			i += 2
		case 'u':
			// \uXXXX
			if i+6 <= len(input) {
				hex := input[i+2 : i+6]
				if r, ok := parseHex4(hex); ok {
					buf.WriteRune(r)
					i += 6
					break
				}
			}
			// fallback: keep literal "\u"
			buf.WriteByte('\\')
			buf.WriteByte('u')
			i += 2
		default:
			// Unknown escape; keep as literal.
			buf.WriteByte(esc)
			i += 2
		}
	}

	return "", start, false
}

func parseHex4(hex string) (rune, bool) {
	if len(hex) != 4 {
		return 0, false
	}
	v := 0
	for i := 0; i < 4; i++ {
		v2, ok := fromHex(hex[i])
		if !ok {
			return 0, false
		}
		v = (v << 4) | v2
	}

	// Handle surrogate pairs produced by some encoders.
	r := rune(v)
	if utf16.IsSurrogate(r) {
		return '\uFFFD', true
	}
	return r, true
}

func fromHex(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	default:
		return 0, false
	}
}

func quoteUnquotedObjectKeys(input string) (string, bool) {
	var b strings.Builder
	b.Grow(len(input))

	inString := false
	quote := byte(0)
	escape := false
	changed := false

	var stack []byte // 'o' for object, 'a' for array

	for i := 0; i < len(input); {
		c := input[i]

		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
				i++
				continue
			}
			if c == '\\' {
				escape = true
				i++
				continue
			}
			if c == quote {
				inString = false
				quote = 0
			}
			i++
			continue
		}

		if c == '"' || c == '\'' {
			inString = true
			quote = c
			b.WriteByte(c)
			i++
			continue
		}

		switch c {
		case '{':
			stack = append(stack, 'o')
			b.WriteByte(c)
			i++
			if len(stack) > 0 && stack[len(stack)-1] == 'o' {
				if nextI, did := tryQuoteKeyAt(input, i, &b); did {
					changed = true
					i = nextI
				}
			}
			continue
		case '[':
			stack = append(stack, 'a')
			b.WriteByte(c)
			i++
			continue
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			b.WriteByte(c)
			i++
			continue
		case ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			b.WriteByte(c)
			i++
			continue
		case ',':
			b.WriteByte(c)
			i++
			if len(stack) > 0 && stack[len(stack)-1] == 'o' {
				if nextI, did := tryQuoteKeyAt(input, i, &b); did {
					changed = true
					i = nextI
				}
			}
			continue
		default:
			b.WriteByte(c)
			i++
			continue
		}
	}

	if !changed {
		return input, false
	}
	return b.String(), true
}

func tryQuoteKeyAt(input string, i int, out *strings.Builder) (newI int, changed bool) {
	j := i
	for j < len(input) && isASCIISpace(input[j]) {
		j++
	}
	if j >= len(input) {
		return i, false
	}
	if input[j] == '"' || input[j] == '\'' {
		return i, false
	}
	if !isIdentStart(input[j]) {
		return i, false
	}
	k := j + 1
	for k < len(input) && isIdentChar(input[k]) {
		k++
	}
	m := k
	for m < len(input) && isASCIISpace(input[m]) {
		m++
	}
	if m >= len(input) || input[m] != ':' {
		return i, false
	}

	out.WriteString(input[i:j])
	out.WriteByte('"')
	out.WriteString(input[j:k])
	out.WriteByte('"')
	out.WriteString(input[k:m])
	return m, true
}

func closeUnclosedBrackets(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return input, false
	}

	inString := false
	quote := byte(0)
	escape := false
	var closers []byte

	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == quote {
				inString = false
				quote = 0
			}
			continue
		}

		if c == '"' || c == '\'' {
			inString = true
			quote = c
			continue
		}

		switch c {
		case '{':
			closers = append(closers, '}')
		case '[':
			closers = append(closers, ']')
		case '}':
			if len(closers) > 0 && closers[len(closers)-1] == '}' {
				closers = closers[:len(closers)-1]
			}
		case ']':
			if len(closers) > 0 && closers[len(closers)-1] == ']' {
				closers = closers[:len(closers)-1]
			}
		}
	}

	if len(closers) == 0 {
		return input, false
	}
	if len(closers) > 64 {
		// Guard against pathological inputs.
		return input, false
	}

	var b strings.Builder
	b.Grow(len(trimmed) + len(closers))
	b.WriteString(trimmed)
	for i := len(closers) - 1; i >= 0; i-- {
		b.WriteByte(closers[i])
	}
	return b.String(), true
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func isIdentStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isIdentChar(b byte) bool {
	if isIdentStart(b) {
		return true
	}
	return (b >= '0' && b <= '9') || b == '-' // allow dash in keys
}

// CompactJSON returns a compacted JSON string if input is valid JSON.
// It returns (input, false) when input isn't valid JSON or compaction fails.
func CompactJSON(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, false
	}
	if !json.Valid([]byte(trimmed)) {
		return input, false
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(trimmed)); err != nil {
		return input, false
	}
	return buf.String(), true
}
