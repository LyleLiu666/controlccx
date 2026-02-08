package llm

import (
	"context"
	"strings"

	"controlccx/internal/agentsdk"
)

// Client adapts completion backends into the agentsdk streaming interface.
// If backend supports ChatStreamBackend, deltas are forwarded as they arrive.
type Client struct {
	Backend Backend
}

func (c *Client) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	if c == nil || c.Backend == nil {
		return callback("秘书不可用：未配置可用的 LLM backend。")
	}

	if csb, ok := c.Backend.(ChatStreamBackend); ok {
		return csb.CompleteChatStream(ctx, messages, opts, callback)
	}

	if cb, ok := c.Backend.(ChatBackend); ok {
		out, err := cb.CompleteChat(ctx, messages, opts)
		if err != nil {
			return err
		}
		return callback(out)
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
