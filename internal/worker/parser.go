package worker

import (
	"strings"

	"github.com/goccy/go-json"
)

type parsedLine struct {
	SessionID     string
	AssistantText string
	IsResult      bool
	ToolUses      []parsedToolUse
	ToolResults   []parsedToolResult
}

type parsedToolUse struct {
	ID   string
	Name string
}

type parsedToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func parseClaudeJSONLine(line []byte) (parsedLine, error) {
	var evt struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype,omitempty"`
		SessionID string `json:"session_id,omitempty"`
		Result    string `json:"result,omitempty"`
		Content   string `json:"content,omitempty"`
		Role      string `json:"role,omitempty"`
		Delta     *bool  `json:"delta,omitempty"`
		Message   *struct {
			Role    string `json:"role,omitempty"`
			Content []struct {
				Type      string `json:"type,omitempty"`
				Text      any    `json:"text,omitempty"`
				ID        string `json:"id,omitempty"`
				Name      string `json:"name,omitempty"`
				ToolUseID string `json:"tool_use_id,omitempty"`
				Content   any    `json:"content,omitempty"`
				IsError   bool   `json:"is_error,omitempty"`
			} `json:"content,omitempty"`
		} `json:"message,omitempty"`
	}
	if err := json.Unmarshal(line, &evt); err != nil {
		return parsedLine{}, err
	}

	out := parsedLine{
		SessionID: strings.TrimSpace(evt.SessionID),
		IsResult:  strings.TrimSpace(evt.Type) == "result",
	}
	// Claude stream-json often includes final answer in `result`.
	if evt.Result != "" {
		out.AssistantText = evt.Result
	} else if evt.Role == "assistant" && evt.Content != "" {
		out.AssistantText = evt.Content
	}

	if evt.Message != nil {
		for _, c := range evt.Message.Content {
			switch strings.TrimSpace(c.Type) {
			case "tool_use":
				id := strings.TrimSpace(c.ID)
				if id == "" {
					continue
				}
				out.ToolUses = append(out.ToolUses, parsedToolUse{
					ID:   id,
					Name: strings.TrimSpace(c.Name),
				})
			case "tool_result":
				id := strings.TrimSpace(c.ToolUseID)
				if id == "" {
					continue
				}
				content := strings.TrimSpace(normalizeText(c.Content))
				out.ToolResults = append(out.ToolResults, parsedToolResult{
					ToolUseID: id,
					Content:   content,
					IsError:   c.IsError,
				})
			default:
				// Ignore other content blocks (text/images/etc).
			}
		}
	}
	return out, nil
}

func parseCodexJSONLine(line []byte) (parsedLine, error) {
	// Legacy JSONL format: used by `codex exec --json`.
	{
		var evt struct {
			Type     string          `json:"type"`
			ThreadID string          `json:"thread_id,omitempty"`
			Item     json.RawMessage `json:"item,omitempty"`
		}
		if err := json.Unmarshal(line, &evt); err == nil && strings.TrimSpace(evt.Type) != "" {
			out := parsedLine{SessionID: strings.TrimSpace(evt.ThreadID)}

			if evt.Type != "item.completed" || len(evt.Item) == 0 {
				return out, nil
			}

			var item struct {
				Type string      `json:"type"`
				Text interface{} `json:"text"`
			}
			if err := json.Unmarshal(evt.Item, &item); err != nil {
				return out, nil
			}
			if item.Type != "agent_message" {
				return out, nil
			}
			out.AssistantText = normalizeText(item.Text)
			return out, nil
		}
	}

	// JSON-RPC notifications: used by `codex app-server`.
	var rpc struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &rpc); err != nil {
		return parsedLine{}, err
	}

	switch strings.TrimSpace(rpc.Method) {
	case "thread/started":
		{
			var p struct {
				Thread struct {
					ID string `json:"id"`
				} `json:"thread"`
			}
			if err := json.Unmarshal(rpc.Params, &p); err != nil {
				return parsedLine{}, nil
			}
			return parsedLine{SessionID: strings.TrimSpace(p.Thread.ID)}, nil
		}
	case "item/completed":
		{
			var p struct {
				ThreadID string `json:"threadId"`
				Item     struct {
					Type string      `json:"type"`
					Text interface{} `json:"text"`
				} `json:"item"`
			}
			if err := json.Unmarshal(rpc.Params, &p); err != nil {
				return parsedLine{}, nil
			}
			out := parsedLine{SessionID: strings.TrimSpace(p.ThreadID)}
			if strings.TrimSpace(p.Item.Type) != "agentMessage" {
				return out, nil
			}
			out.AssistantText = normalizeText(p.Item.Text)
			return out, nil
		}
	default:
		return parsedLine{}, nil
	}
}

func normalizeText(text interface{}) string {
	switch v := text.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, item := range v {
			if s, ok := item.(string); ok {
				sb.WriteString(s)
			}
		}
		return sb.String()
	default:
		return ""
	}
}
