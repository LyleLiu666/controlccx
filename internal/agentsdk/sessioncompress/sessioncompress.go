package sessioncompress

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaxContextRunes = 80_000

	// Keep last 2 rounds (user+assistant)*2.
	DefaultKeepTextMessages = 4

	DefaultMaxSummaryInputRunes = 120_000
	DefaultMaxMsgRunesForInput  = 4_000
)

type Message struct {
	ID       uint
	ParentID *uint
	Role     string
	Type     string
	Content  string
}

type FormatMessageContentFunc func(msg Message) string

type Options struct {
	MaxContextRunes      int
	KeepTextMessages     int
	MaxSummaryInputRunes int
	MaxMsgRunesForInput  int
	FormatMessageContent FormatMessageContentFunc
}

func DefaultOptions() Options {
	return Options{
		MaxContextRunes:      DefaultMaxContextRunes,
		KeepTextMessages:     DefaultKeepTextMessages,
		MaxSummaryInputRunes: DefaultMaxSummaryInputRunes,
		MaxMsgRunesForInput:  DefaultMaxMsgRunesForInput,
		FormatMessageContent: func(msg Message) string {
			return DefaultFormatMessageContent(msg)
		},
	}
}

func normalizeOptions(opts Options) Options {
	def := DefaultOptions()
	if opts.MaxContextRunes <= 0 {
		opts.MaxContextRunes = def.MaxContextRunes
	}
	if opts.KeepTextMessages < 0 {
		opts.KeepTextMessages = def.KeepTextMessages
	}
	if opts.MaxSummaryInputRunes <= 0 {
		opts.MaxSummaryInputRunes = def.MaxSummaryInputRunes
	}
	if opts.MaxMsgRunesForInput <= 0 {
		opts.MaxMsgRunesForInput = def.MaxMsgRunesForInput
	}
	if opts.FormatMessageContent == nil {
		opts.FormatMessageContent = def.FormatMessageContent
	}
	return opts
}

func ApproximateContextRunes(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += utf8.RuneCountInString(msg.Role)
		total += utf8.RuneCountInString(msg.Type)
		total += utf8.RuneCountInString(msg.Content)
	}
	return total
}

// FindKeepStartID selects the start id of the kept tail (messages with id >= keepStart are kept).
// It always returns a value >= cursor+1. If keepTextMessages <= 0, it keeps no messages.
//
// Closure fix:
// If any kept message references a parent with id < keepStart (but > cursor), lower keepStart
// until all parent chains are included.
func FindKeepStartID(messages []Message, cursor uint, keepTextMessages int) uint {
	lastID := uint(0)
	for _, msg := range messages {
		if msg.ID > lastID {
			lastID = msg.ID
		}
	}

	hasAfterCursor := false
	for _, msg := range messages {
		if msg.ID > cursor {
			hasAfterCursor = true
			break
		}
	}
	if !hasAfterCursor {
		return cursor + 1
	}

	if keepTextMessages <= 0 {
		return lastID + 1
	}

	keepIDs := make(map[uint]struct{}, keepTextMessages)
	for i := len(messages) - 1; i >= 0 && len(keepIDs) < keepTextMessages; i-- {
		msg := messages[i]
		if msg.ID <= cursor {
			continue
		}
		if msg.ParentID != nil {
			continue
		}
		if strings.TrimSpace(msg.Type) != "" && msg.Type != "text" {
			continue
		}
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		keepIDs[msg.ID] = struct{}{}
	}

	keepStart := uint(0)
	if len(keepIDs) == 0 {
		keepStart = lastID + 1
	} else {
		for id := range keepIDs {
			if keepStart == 0 || id < keepStart {
				keepStart = id
			}
		}
	}
	if keepStart <= cursor+1 {
		keepStart = cursor + 1
	}

	// Closure fix: include parents for all kept messages, bounded by cursor.
	for {
		changed := false
		for _, msg := range messages {
			if msg.ID < keepStart {
				continue
			}
			if msg.ParentID == nil {
				continue
			}
			parentID := *msg.ParentID
			if parentID <= cursor {
				continue
			}
			if parentID < keepStart {
				keepStart = parentID
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}

	if keepStart <= cursor+1 {
		keepStart = cursor + 1
	}
	return keepStart
}

func SplitForCompression(messages []Message, cursor uint, keepTextMessages int) (toSummarize []Message, toKeep []Message, newCursor uint, keepFromID uint) {
	keepStart := FindKeepStartID(messages, cursor, keepTextMessages)
	if keepStart <= cursor+1 {
		newCursor = cursor
		keepFromID = cursor + 1
	} else {
		newCursor = keepStart - 1
		keepFromID = keepStart
	}

	toSummarize = make([]Message, 0, len(messages))
	toKeep = make([]Message, 0, len(messages))
	for _, msg := range messages {
		if msg.ID <= cursor {
			continue
		}
		if msg.ID <= newCursor {
			toSummarize = append(toSummarize, msg)
			continue
		}
		toKeep = append(toKeep, msg)
	}

	return toSummarize, toKeep, newCursor, keepFromID
}

func FormatForSummaryInput(messages []Message, opts Options) string {
	opts = normalizeOptions(opts)
	var b strings.Builder

	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "unknown"
		}
		msgType := strings.TrimSpace(msg.Type)
		if msgType == "" {
			msgType = "text"
		}

		content := strings.TrimSpace(opts.FormatMessageContent(msg))
		content = truncateString(content, opts.MaxMsgRunesForInput)
		if content == "" {
			continue
		}

		fmt.Fprintf(&b, "[%d][%s][%s] %s\n", msg.ID, role, msgType, content)
	}

	out := b.String()
	if out == "" {
		return out
	}

	if utf8.RuneCountInString(out) > opts.MaxSummaryInputRunes {
		out = truncateString(out, opts.MaxSummaryInputRunes)
	}
	return out
}

func DefaultFormatMessageContent(msg Message) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	switch strings.TrimSpace(msg.Type) {
	case "", "text":
		return content
	case "tool_call":
		var payload struct {
			AgentID  string          `json:"agent_id,omitempty"`
			ToolName string          `json:"tool_name"`
			Input    json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}
		args := strings.TrimSpace(string(payload.Input))
		if args == "" {
			args = "null"
		}
		if payload.AgentID != "" {
			return fmt.Sprintf("tool_call agent=%s tool=%s input=%s", payload.AgentID, payload.ToolName, args)
		}
		return fmt.Sprintf("tool_call tool=%s input=%s", payload.ToolName, args)

	case "tool_result":
		var payload struct {
			AgentID  string `json:"agent_id,omitempty"`
			ToolName string `json:"tool_name"`
			Output   string `json:"output,omitempty"`
			Error    string `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}
		parts := []string{fmt.Sprintf("tool_result tool=%s", payload.ToolName)}
		if strings.TrimSpace(payload.Error) != "" {
			parts = append(parts, "error="+strings.TrimSpace(payload.Error))
		}
		if strings.TrimSpace(payload.Output) != "" {
			parts = append(parts, "output="+strings.TrimSpace(payload.Output))
		}
		if payload.AgentID != "" {
			parts = append(parts, "agent="+payload.AgentID)
		}
		return strings.Join(parts, " ")

	case "subagent_call":
		var payload struct {
			SubAgentID string `json:"subagent_id"`
			Message    string `json:"message"`
		}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}
		return fmt.Sprintf("subagent_call id=%s message=%s", payload.SubAgentID, strings.TrimSpace(payload.Message))

	case "subagent_result":
		var payload struct {
			SubAgentID string `json:"subagent_id"`
			Output     string `json:"output,omitempty"`
			Error      string `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return content
		}
		parts := []string{fmt.Sprintf("subagent_result id=%s", payload.SubAgentID)}
		if strings.TrimSpace(payload.Error) != "" {
			parts = append(parts, "error="+strings.TrimSpace(payload.Error))
		}
		if strings.TrimSpace(payload.Output) != "" {
			parts = append(parts, "output="+strings.TrimSpace(payload.Output))
		}
		return strings.Join(parts, " ")

	default:
		return content
	}
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	return string(runes[:maxLen])
}
