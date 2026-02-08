package worker

import "testing"

func TestParseClaudeJSONLine(t *testing.T) {
	line := []byte(`{"type":"result","session_id":"abc123","result":"Hello"}`)
	parsed, err := parseClaudeJSONLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.SessionID != "abc123" {
		t.Fatalf("session_id=%q, want abc123", parsed.SessionID)
	}
	if parsed.AssistantText != "Hello" {
		t.Fatalf("assistant=%q, want Hello", parsed.AssistantText)
	}
}

func TestParseCodexJSONLine(t *testing.T) {
	t.Run("thread started", func(t *testing.T) {
		line := []byte(`{"type":"thread.started","thread_id":"tid-1"}`)
		parsed, err := parseCodexJSONLine(line)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.SessionID != "tid-1" {
			t.Fatalf("session_id=%q, want tid-1", parsed.SessionID)
		}
	})

	t.Run("agent_message string", func(t *testing.T) {
		line := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"hi"}}`)
		parsed, err := parseCodexJSONLine(line)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.AssistantText != "hi" {
			t.Fatalf("assistant=%q, want hi", parsed.AssistantText)
		}
	})

	t.Run("agent_message array", func(t *testing.T) {
		line := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":["a","b"]}}`)
		parsed, err := parseCodexJSONLine(line)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.AssistantText != "ab" {
			t.Fatalf("assistant=%q, want ab", parsed.AssistantText)
		}
	})

	t.Run("rpc thread started", func(t *testing.T) {
		line := []byte(`{"method":"thread/started","params":{"thread":{"id":"thr-1"}}}`)
		parsed, err := parseCodexJSONLine(line)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.SessionID != "thr-1" {
			t.Fatalf("session_id=%q, want thr-1", parsed.SessionID)
		}
	})

	t.Run("rpc item completed agentMessage", func(t *testing.T) {
		line := []byte(`{"method":"item/completed","params":{"threadId":"thr-1","turnId":"turn-1","item":{"id":"it-1","type":"agentMessage","text":"hello"}}}`)
		parsed, err := parseCodexJSONLine(line)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.SessionID != "thr-1" {
			t.Fatalf("session_id=%q, want thr-1", parsed.SessionID)
		}
		if parsed.AssistantText != "hello" {
			t.Fatalf("assistant=%q, want hello", parsed.AssistantText)
		}
	})
}
