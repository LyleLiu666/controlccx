package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/providers"

	"github.com/goccy/go-json"
)

const (
	defaultSimpleHTTPBaseURL = "https://api.anthropic.com"
	defaultSimpleHTTPModel   = "claude-3-5-sonnet-latest"
	defaultSimpleHTTPTimeout = 60 * time.Second
	defaultSimpleHTTPTokens  = 2048
)

type SimpleHTTPBackend struct {
	cfg       config.Config
	auth      *auth.Store
	providers *providers.Store
	client    *http.Client
}

func NewSimpleHTTPBackend(cfg config.Config, authStore *auth.Store) Backend {
	return NewSimpleHTTPBackendWithProviders(cfg, authStore, nil)
}

func NewSimpleHTTPBackendWithProviders(cfg config.Config, authStore *auth.Store, providersStore *providers.Store) Backend {
	return &SimpleHTTPBackend{
		cfg:       cfg,
		auth:      authStore,
		providers: providersStore,
		client:    &http.Client{},
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

	authToken := strings.TrimSpace(b.resolveAnthropicAuthToken())
	apiKey := strings.TrimSpace(b.resolveAnthropicAPIKey())
	var live map[string]string
	if authToken == "" && apiKey == "" {
		live = readClaudeLiveEnvBestEffort()
		if baseURL == defaultSimpleHTTPBaseURL {
			if v := strings.TrimSpace(live["ANTHROPIC_BASE_URL"]); v != "" {
				baseURL = v
			}
		}
		if v := strings.TrimSpace(live["ANTHROPIC_AUTH_TOKEN"]); v != "" {
			authToken = v
		}
		if v := strings.TrimSpace(live["ANTHROPIC_API_KEY"]); v != "" {
			apiKey = v
		}
		if authToken == "" && apiKey == "" {
			return "", fmt.Errorf("simple-http missing credentials: set ANTHROPIC_AUTH_TOKEN (preferred) or ANTHROPIC_API_KEY (or run `claude /login` to generate ~/.claude/settings.json)")
		}
	}

	model := strings.TrimSpace(b.resolveAnthropicModel())
	if model == "" {
		if live == nil {
			live = readClaudeLiveEnvBestEffort()
		}
		model = strings.TrimSpace(live["ANTHROPIC_MODEL"])
		if model == "" {
			model = defaultSimpleHTTPModel
		}
	}

	endpoint, err := normalizeMessagesEndpoint(baseURL)
	if err != nil {
		return "", err
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

func readClaudeLiveEnvBestEffort() map[string]string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	dir := filepath.Join(filepath.Clean(home), ".claude")
	path := ""
	for _, name := range []string{"settings.json", "claude.json"} {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil || info == nil || info.IsDir() || info.Size() <= 0 {
			continue
		}
		path = p
		break
	}
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	raw, ok := v["env"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, vv := range raw {
		ks := strings.TrimSpace(k)
		if ks == "" {
			continue
		}
		s, ok := vv.(string)
		if !ok {
			continue
		}
		out[ks] = s
	}
	return out
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
	if v := strings.TrimSpace(b.resolveSecretarySimpleHTTP().BaseURL); v != "" {
		return v
	}
	if b.auth != nil {
		if v := strings.TrimSpace(b.auth.Get().AnthropicBaseURL); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")); v != "" {
		return v
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicAPIKey() string {
	if v := strings.TrimSpace(b.resolveSecretarySimpleHTTP().APIKey); v != "" {
		return v
	}
	if b.auth != nil {
		if v := strings.TrimSpace(b.auth.Get().AnthropicAPIKey); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); v != "" {
		return v
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicAuthToken() string {
	if v := strings.TrimSpace(b.resolveSecretarySimpleHTTP().AuthToken); v != "" {
		return v
	}
	if b.auth != nil {
		if v := strings.TrimSpace(b.auth.Get().AnthropicAuthToken); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")); v != "" {
		return v
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveAnthropicModel() string {
	if v := strings.TrimSpace(b.resolveSecretarySimpleHTTP().Model); v != "" {
		return v
	}
	if b.auth != nil {
		if v := strings.TrimSpace(b.auth.Get().AnthropicModel); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")); v != "" {
		return v
	}
	return ""
}

func (b *SimpleHTTPBackend) resolveSecretarySimpleHTTP() providers.SecretarySimpleHTTP {
	if b == nil || b.providers == nil {
		return providers.SecretarySimpleHTTP{}
	}
	active := b.providers.Active()
	id := strings.TrimSpace(active.Secretary)
	if id == "" {
		return providers.SecretarySimpleHTTP{}
	}
	p, ok := b.providers.Get(id)
	if !ok {
		return providers.SecretarySimpleHTTP{}
	}
	return p.Targets.Secretary.SimpleHTTP
}
