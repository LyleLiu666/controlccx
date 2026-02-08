package llm

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

// Client adapts a completion-style backend into the agentsdk streaming interface.
// It emits the full completion as a single chunk (no token streaming).
type Client struct {
	Backend Backend
}

func (c *Client) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = opts
	if c == nil || c.Backend == nil {
		return callback("秘书不可用：未配置可用的 LLM backend。")
	}

	prompt := flattenMessages(messages)
	out, err := c.Backend.Complete(ctx, prompt)
	if err != nil {
		return err
	}
	return callback(out)
}

func flattenMessages(messages []agentsdk.Message) string {
	if len(messages) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, m := range messages {
		role := strings.ToUpper(strings.TrimSpace(m.Role))
		if role == "" {
			role = "MESSAGE"
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(role)
		sb.WriteString(":\n")
		sb.WriteString(content)
	}
	return strings.TrimSpace(sb.String())
}
