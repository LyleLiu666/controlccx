package observer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"

	"github.com/goccy/go-json"
)

const (
	defaultSimpleHTTPBaseURL = "https://api.anthropic.com"
	defaultSimpleHTTPModel   = "claude-3-5-sonnet-latest"
	defaultSimpleHTTPTimeout = 60 * time.Second
	defaultSimpleHTTPTokens  = 2048
)

type SimpleHTTPBackend struct {
	cfg    config.Config
	auth   *auth.Store
	client *http.Client
}

func NewSimpleHTTPBackend(cfg config.Config, authStore *auth.Store) Backend {
	return &SimpleHTTPBackend{
		cfg:    cfg,
		auth:   authStore,
		client: &http.Client{},
	}
}

func (b *SimpleHTTPBackend) Name() string { return "simple-http" }

func (b *SimpleHTTPBackend) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := withDefaultTimeout(ctx, defaultSimpleHTTPTimeout)
	defer cancel()

	baseURL := strings.TrimSpace(b.resolveAnthropicBaseURL())
	if baseURL == "" {
		baseURL = defaultSimpleHTTPBaseURL
	}
	endpoint, err := normalizeMessagesEndpoint(baseURL)
	if err != nil {
		return "", err
	}

	authToken := strings.TrimSpace(b.resolveAnthropicAuthToken())
	apiKey := strings.TrimSpace(b.resolveAnthropicAPIKey())
	if authToken == "" && apiKey == "" {
		return "", fmt.Errorf("simple-http missing credentials: set ANTHROPIC_AUTH_TOKEN (preferred) or ANTHROPIC_API_KEY")
	}

	model := strings.TrimSpace(b.resolveAnthropicModel())
	if model == "" {
		model = defaultSimpleHTTPModel
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": defaultSimpleHTTPTokens,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("simple-http marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("simple-http build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if authToken != "" {
		// Compatibility-first: many gateways accept Bearer; some accept x-api-key style.
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("x-api-key", authToken)
	} else {
		req.Header.Set("x-api-key", apiKey)
	}

	client := b.client
	if client == nil {
		client = &http.Client{}
	}

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("simple-http request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("simple-http read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := extractErrorMessage(raw)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return "", fmt.Errorf("simple-http status %d: %s", res.StatusCode, msg)
	}

	text := extractCompletionText(raw)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("simple-http empty completion from %s", endpoint)
	}
	return strings.TrimSpace(text), nil
}

func normalizeMessagesEndpoint(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("simple-http base url is empty")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("simple-http invalid base url %q", base)
	}
	path := strings.TrimSpace(u.Path)
	if strings.HasSuffix(path, "/v1/messages") || path == "/v1/messages" {
		u.Path = ensureLeadingSlash(path)
		return u.String(), nil
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		u.Path = "/v1/messages"
	} else {
		u.Path = path + "/v1/messages"
	}
	return u.String(), nil
}

func ensureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func extractErrorMessage(raw []byte) string {
	type errorBody struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	var out errorBody
	if err := json.Unmarshal(raw, &out); err != nil {
		return ""
	}
	if strings.TrimSpace(out.Error.Message) != "" {
		return strings.TrimSpace(out.Error.Message)
	}
	return strings.TrimSpace(out.Message)
}

func extractCompletionText(raw []byte) string {
	type anthropicContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type anthropicResponse struct {
		Content []anthropicContent `json:"content"`
	}
	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err == nil && len(ar.Content) > 0 {
		var sb strings.Builder
		for _, c := range ar.Content {
			if strings.TrimSpace(c.Text) == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(strings.TrimSpace(c.Text))
		}
		if strings.TrimSpace(sb.String()) != "" {
			return strings.TrimSpace(sb.String())
		}
	}

	type openAIChoice struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	}
	type openAIResponse struct {
		Choices []openAIChoice `json:"choices"`
	}
	var or openAIResponse
	if err := json.Unmarshal(raw, &or); err == nil && len(or.Choices) > 0 {
		var sb strings.Builder
		for _, c := range or.Choices {
			if text := textFromAny(c.Message.Content); strings.TrimSpace(text) != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(strings.TrimSpace(text))
				continue
			}
			if strings.TrimSpace(c.Text) != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(strings.TrimSpace(c.Text))
			}
		}
		if strings.TrimSpace(sb.String()) != "" {
			return strings.TrimSpace(sb.String())
		}
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return ""
	}
	if s := textFromAny(generic["output_text"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := textFromAny(generic["result"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := textFromAny(generic["text"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := textFromAny(generic["message"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return ""
}

func textFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var sb strings.Builder
		for _, item := range x {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(strings.TrimSpace(textFromAny(item)))
		}
		return strings.TrimSpace(sb.String())
	case map[string]any:
		if t, ok := x["text"]; ok {
			return textFromAny(t)
		}
		if c, ok := x["content"]; ok {
			return textFromAny(c)
		}
		return ""
	default:
		return ""
	}
}

func (b *SimpleHTTPBackend) resolveAnthropicBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")); v != "" {
		return v
	}
	if b.auth != nil {
		return strings.TrimSpace(b.auth.Get().AnthropicBaseURL)
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); v != "" {
		return v
	}
	if b.auth != nil {
		return strings.TrimSpace(b.auth.Get().AnthropicAPIKey)
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicAuthToken() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")); v != "" {
		return v
	}
	if b.auth != nil {
		return strings.TrimSpace(b.auth.Get().AnthropicAuthToken)
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicModel() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")); v != "" {
		return v
	}
	if b.auth != nil {
		return strings.TrimSpace(b.auth.Get().AnthropicModel)
	}
	return ""
}
