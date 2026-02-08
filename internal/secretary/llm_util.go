package secretary

import (
	"context"
	"errors"
	"strings"

	"controlccx/internal/agentsdk"
)

func completeText(ctx context.Context, client agentsdk.Client, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	if client == nil {
		return "", errors.New("missing llm client")
	}
	var b strings.Builder
	err := client.ChatCompletionStream(ctx, messages, opts, func(chunk string) error {
		b.WriteString(chunk)
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
