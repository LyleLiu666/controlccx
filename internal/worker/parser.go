package worker

import (
	"strings"

	"github.com/goccy/go-json"
)

type parsedLine struct {
	SessionID     string
	AssistantText string
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
	}
	if err := json.Unmarshal(line, &evt); err != nil {
		return parsedLine{}, err
	}

	out := parsedLine{SessionID: strings.TrimSpace(evt.SessionID)}
	// Claude stream-json often includes final answer in `result`.
	if evt.Result != "" {
		out.AssistantText = evt.Result
	} else if evt.Role == "assistant" && evt.Content != "" {
		out.AssistantText = evt.Content
	}
	return out, nil
}

func parseCodexJSONLine(line []byte) (parsedLine, error) {
	var evt struct {
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id,omitempty"`
		Item     json.RawMessage `json:"item,omitempty"`
	}
	if err := json.Unmarshal(line, &evt); err != nil {
		return parsedLine{}, err
	}

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

