package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultPingTestTimeout  = 8 * time.Second
	defaultPingTestMaxTokens = 16
	defaultPingTestModel     = "claude-3-5-sonnet-latest"
)

type PingTestOptions struct {
	Timeout   time.Duration
	Client    *http.Client // optional; useful for tests (e.g. TLS server)
	MaxTokens int
	Prompt    string
}

type PingTestResult struct {
	Endpoint   string `json:"endpoint"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Response   string `json:"response,omitempty"`
	Error      string `json:"error,omitempty"`
	Hint       string `json:"hint,omitempty"`
}

func PingTest(ctx context.Context, cfg SecretarySimpleHTTP, opts PingTestOptions) PingTestResult {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return PingTestResult{Endpoint: baseURL, OK: false, Hint: "invalid_url", Error: "base url is required"}
	}

	endpoint, err := normalizeAnthropicMessagesEndpoint(baseURL)
	if err != nil {
		return PingTestResult{Endpoint: baseURL, OK: false, Hint: "invalid_url", Error: err.Error()}
	}

	authToken := strings.TrimSpace(cfg.AuthToken)
	apiKey := strings.TrimSpace(cfg.APIKey)
	if authToken == "" && apiKey == "" {
		return PingTestResult{Endpoint: endpoint, OK: false, Hint: "missing_credentials", Error: "missing credentials"}
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultPingTestModel
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultPingTestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultPingTestMaxTokens
	}
	if maxTokens > 512 {
		maxTokens = 512
	}

	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		prompt = "ping\n\nReply with the single word 'pong'."
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return PingTestResult{Endpoint: endpoint, OK: false, Hint: "internal_error", Error: fmt.Errorf("marshal request: %w", err).Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return PingTestResult{Endpoint: endpoint, OK: false, Hint: "internal_error", Error: fmt.Errorf("build request: %w", err).Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("x-api-key", authToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
	}

	client := opts.Client
	if client == nil {
		client = &http.Client{}
	}

	start := time.Now()
	res, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return PingTestResult{
			Endpoint:  endpoint,
			OK:        false,
			LatencyMS: latency.Milliseconds(),
			Error:     err.Error(),
			Hint:      hintFromSpeedTestError(err),
		}
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return PingTestResult{Endpoint: endpoint, OK: false, StatusCode: res.StatusCode, LatencyMS: latency.Milliseconds(), Hint: "network_error", Error: fmt.Errorf("read response: %w", err).Error()}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := extractPingTestErrorMessage(raw)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return PingTestResult{
			Endpoint:   endpoint,
			OK:         false,
			StatusCode: res.StatusCode,
			LatencyMS:  latency.Milliseconds(),
			Error:      msg,
			Hint:       "bad_status",
		}
	}

	text := strings.TrimSpace(extractPingTestCompletionText(raw))
	if text == "" {
		return PingTestResult{
			Endpoint:   endpoint,
			OK:         false,
			StatusCode: res.StatusCode,
			LatencyMS:  latency.Milliseconds(),
			Error:      "empty completion",
			Hint:       "empty_response",
		}
	}
	return PingTestResult{
		Endpoint:   endpoint,
		OK:         true,
		StatusCode: res.StatusCode,
		LatencyMS:  latency.Milliseconds(),
		Response:   text,
	}
}

func normalizeAnthropicMessagesEndpoint(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("base url is empty")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base url %q", base)
	}
	path := strings.TrimSpace(u.Path)
	if strings.HasSuffix(path, "/v1/messages") || path == "/v1/messages" {
		u.Path = pingEnsureLeadingSlash(path)
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

func pingEnsureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func extractPingTestErrorMessage(raw []byte) string {
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

func extractPingTestCompletionText(raw []byte) string {
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
			if text := pingTextFromAny(c.Message.Content); strings.TrimSpace(text) != "" {
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
	if s := pingTextFromAny(generic["output_text"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := pingTextFromAny(generic["result"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := pingTextFromAny(generic["text"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	if s := pingTextFromAny(generic["message"]); strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s)
	}
	return ""
}

func pingTextFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var sb strings.Builder
		for _, item := range x {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(strings.TrimSpace(pingTextFromAny(item)))
		}
		return strings.TrimSpace(sb.String())
	case map[string]any:
		if t, ok := x["text"]; ok {
			return pingTextFromAny(t)
		}
		if c, ok := x["content"]; ok {
			return pingTextFromAny(c)
		}
		return ""
	default:
		return ""
	}
}
