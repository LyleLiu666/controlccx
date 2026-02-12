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

func TestParseClaudeJSONLine_ToolUseAndToolResult(t *testing.T) {
	use := []byte(`{"type":"assistant","session_id":"sess-1","message":{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"Bash","input":{"cmd":"echo hi"}}]}}`)
	parsed, err := parseClaudeJSONLine(use)
	if err != nil {
		t.Fatalf("parse tool_use: %v", err)
	}
	if len(parsed.ToolUses) != 1 || parsed.ToolUses[0].ID != "call-1" || parsed.ToolUses[0].Name != "Bash" {
		t.Fatalf("tool_uses=%+v", parsed.ToolUses)
	}

	result := []byte(`{"type":"user","session_id":"sess-1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":["Exit code 1\\n","mkdir: Operation not permitted\\n"],"is_error":true}]}}`)
	parsed, err = parseClaudeJSONLine(result)
	if err != nil {
		t.Fatalf("parse tool_result: %v", err)
	}
	if len(parsed.ToolResults) != 1 {
		t.Fatalf("tool_results=%+v", parsed.ToolResults)
	}
	if parsed.ToolResults[0].ToolUseID != "call-1" {
		t.Fatalf("tool_use_id=%q want %q", parsed.ToolResults[0].ToolUseID, "call-1")
	}
	if !parsed.ToolResults[0].IsError {
		t.Fatalf("is_error=false want true")
	}
	if parsed.ToolResults[0].Content == "" {
		t.Fatalf("expected non-empty content")
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
