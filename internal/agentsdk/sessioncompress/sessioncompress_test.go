package sessioncompress

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFindKeepStartID_ClosureFixIncludesParent(t *testing.T) {
	parentID := uint(3)
	msgs := []Message{
		{ID: 1, Role: "user", Type: "text", Content: "u1"},
		{ID: 2, Role: "assistant", Type: "text", Content: "a1"},
		{ID: 3, Role: "tool", Type: "tool_call", Content: `{"tool_name":"t","input":{"x":1}}`},
		{ID: 4, Role: "assistant", Type: "text", Content: "a2"},
		{ID: 5, ParentID: &parentID, Role: "tool", Type: "tool_result", Content: `{"tool_name":"t","output":"ok"}`},
		{ID: 6, Role: "user", Type: "text", Content: "u2"},
		{ID: 7, Role: "assistant", Type: "text", Content: "a3"},
	}

	// Keep last 3 text messages => keepStart would be 4, but msg[5] depends on parent 3.
	got := FindKeepStartID(msgs, 0, 3)
	if got != 3 {
		t.Fatalf("keepStart want 3, got %d", got)
	}
}

func TestSplitForCompression_NewCursorAndKeepFrom(t *testing.T) {
	msgs := []Message{
		{ID: 1, Role: "user", Type: "text", Content: "u1"},
		{ID: 2, Role: "assistant", Type: "text", Content: "a1"},
		{ID: 3, Role: "user", Type: "text", Content: "u2"},
		{ID: 4, Role: "assistant", Type: "text", Content: "a2"},
	}

	toSummarize, toKeep, newCursor, keepFrom := SplitForCompression(msgs, 0, 2)
	if newCursor != 2 {
		t.Fatalf("newCursor want 2, got %d", newCursor)
	}
	if keepFrom != 3 {
		t.Fatalf("keepFrom want 3, got %d", keepFrom)
	}
	if len(toSummarize) != 2 || toSummarize[0].ID != 1 || toSummarize[1].ID != 2 {
		t.Fatalf("toSummarize mismatch: %#v", toSummarize)
	}
	if len(toKeep) != 2 || toKeep[0].ID != 3 || toKeep[1].ID != 4 {
		t.Fatalf("toKeep mismatch: %#v", toKeep)
	}
}

func TestFormatForSummaryInput_TruncatesPerMessageAndHardCapsTotal(t *testing.T) {
	msgs := []Message{
		{ID: 1, Role: "user", Type: "text", Content: "hello"},
		{ID: 2, Role: "assistant", Type: "text", Content: strings.Repeat("a", 20)},
	}

	out := FormatForSummaryInput(msgs, Options{
		MaxSummaryInputRunes: 200,
		MaxMsgRunesForInput:  4,
		FormatMessageContent: func(msg Message) string {
			return msg.Content
		},
	})
	if !strings.Contains(out, "[1][user][text] hell") {
		t.Fatalf("expected truncated line for msg1, got %q", out)
	}
	if !strings.Contains(out, "[2][assistant][text] aaaa") {
		t.Fatalf("expected truncated line for msg2, got %q", out)
	}

	capped := FormatForSummaryInput(msgs, Options{
		MaxSummaryInputRunes: 20,
		MaxMsgRunesForInput:  100,
		FormatMessageContent: func(msg Message) string {
			return msg.Content
		},
	})
	if utf8.RuneCountInString(capped) > 20 {
		t.Fatalf("expected hard cap <=20 runes, got %d: %q", utf8.RuneCountInString(capped), capped)
	}
}
