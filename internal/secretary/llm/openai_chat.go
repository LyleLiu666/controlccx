package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"controlccx/internal/agentsdk"
	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/providers"

	"github.com/goccy/go-json"
)

const (
	defaultOpenAIChatBaseURL = "https://api.openai.com"
	defaultOpenAIChatModel   = "gpt-4o-mini"
	defaultOpenAIChatTokens  = 2048
)

type OpenAIChatBackend struct {
	cfg       config.Config
	auth      *auth.Store
	providers *providers.Store
	client    *http.Client

	receiptMu   sync.RWMutex
	lastReceipt map[string]any
}

func NewOpenAIChatBackend(cfg config.Config, authStore *auth.Store) Backend {
	return NewOpenAIChatBackendWithProviders(cfg, authStore, nil)
}

func NewOpenAIChatBackendWithProviders(cfg config.Config, authStore *auth.Store, providersStore *providers.Store) Backend {
	return &OpenAIChatBackend{
		cfg:       cfg,
		auth:      authStore,
		providers: providersStore,
		client:    &http.Client{},
	}
}

func (b *OpenAIChatBackend) Name() string { return "openai-chat" }

func (b *OpenAIChatBackend) LastReceipt() map[string]any {
	if b == nil {
		return nil
	}
	b.receiptMu.RLock()
	defer b.receiptMu.RUnlock()
	return cloneAnyMap(b.lastReceipt)
}

func (b *OpenAIChatBackend) setLastReceipt(receipt map[string]any) {
	if b == nil {
		return
	}
	b.receiptMu.Lock()
	defer b.receiptMu.Unlock()
	b.lastReceipt = cloneAnyMap(receipt)
}

func (b *OpenAIChatBackend) Complete(ctx context.Context, prompt string) (string, error) {
	return b.CompleteChat(ctx, []agentsdk.Message{{Role: "user", Content: prompt}}, nil)
}

func (b *OpenAIChatBackend) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	return b.completeChat(ctx, messages, opts, nil)
}

func (b *OpenAIChatBackend) CompleteChatStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_, err := b.completeChat(ctx, messages, opts, callback)
	return err
}

func (b *OpenAIChatBackend) completeChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) (string, error) {
	b.setLastReceipt(nil)

	timeout := resolveSecretaryLLMTimeout(b.cfg)
	ctx, cancel := withDefaultTimeout(ctx, timeout)
	defer cancel()

	baseURL := strings.TrimSpace(b.resolveOpenAIBaseURL())
	if baseURL == "" {
		baseURL = defaultOpenAIChatBaseURL
	}

	apiKey := strings.TrimSpace(b.resolveOpenAIAPIKey())
	if apiKey == "" {
		return "", fmt.Errorf("openai-chat missing credentials: set OPENAI_API_KEY (or configure Providers → 秘书 → openai-chat API key)")
	}

	model := strings.TrimSpace(b.resolveOpenAIModel())
	if model == "" {
		model = defaultOpenAIChatModel
	}
	if opts != nil && strings.TrimSpace(opts.Model) != "" {
		model = strings.TrimSpace(opts.Model)
	}

	endpoint, err := normalizeChatCompletionsEndpoint(baseURL)
	if err != nil {
		return "", err
	}

	maxTokens := defaultOpenAIChatTokens
	if opts != nil && opts.MaxTokens != nil && *opts.MaxTokens > 0 {
		maxTokens = *opts.MaxTokens
	}

	reqBody := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   buildOpenAIChatMessages(messages),
	}
	if opts != nil {
		if opts.Temperature != nil {
			reqBody["temperature"] = *opts.Temperature
		}
		if len(opts.Stop) > 0 {
			reqBody["stop"] = append([]string(nil), opts.Stop...)
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openai-chat marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("openai-chat build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := b.client
	if client == nil {
		client = &http.Client{}
	}

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai-chat request failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("openai-chat read response: %w", err)
	}

	receipt := buildOpenAIChatReceipt(res, raw, model)
	b.setLastReceipt(receipt)

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := extractErrorMessage(raw)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = http.StatusText(res.StatusCode)
		}
		return "", fmt.Errorf("openai-chat status %d: %s", res.StatusCode, msg)
	}

	text := strings.TrimSpace(extractCompletionText(raw))
	if text == "" {
		return "", fmt.Errorf("openai-chat empty completion from %s", endpoint)
	}
	if callback != nil {
		if err := callback(text); err != nil {
			return "", err
		}
	}
	return text, nil
}

func buildOpenAIChatMessages(messages []agentsdk.Message) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if role == "" {
			role = "user"
		}
		out = append(out, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func normalizeChatCompletionsEndpoint(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", fmt.Errorf("openai-chat base url is empty")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("openai-chat invalid base url %q", base)
	}
	path := strings.TrimSpace(u.Path)
	if strings.HasSuffix(path, "/v1/chat/completions") || path == "/v1/chat/completions" {
		u.Path = ensureLeadingSlash(path)
		return u.String(), nil
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		u.Path = "/v1/chat/completions"
	} else {
		u.Path = path + "/v1/chat/completions"
	}
	return u.String(), nil
}

func (b *OpenAIChatBackend) resolveOpenAIBaseURL() string {
	if cfg := b.resolveSecretaryOpenAIChat(); strings.TrimSpace(cfg.BaseURL) != "" {
		return strings.TrimSpace(cfg.BaseURL)
	}
	return strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
}

func (b *OpenAIChatBackend) resolveOpenAIAPIKey() string {
	if cfg := b.resolveSecretaryOpenAIChat(); strings.TrimSpace(cfg.APIKey) != "" {
		return strings.TrimSpace(cfg.APIKey)
	}
	if b != nil && b.auth != nil {
		if v := strings.TrimSpace(b.auth.Get().OpenAIAPIKey); v != "" {
			return v
		}
	}
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}

func (b *OpenAIChatBackend) resolveOpenAIModel() string {
	if cfg := b.resolveSecretaryOpenAIChat(); strings.TrimSpace(cfg.Model) != "" {
		return strings.TrimSpace(cfg.Model)
	}
	return strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
}

func (b *OpenAIChatBackend) resolveSecretaryOpenAIChat() providers.SecretaryOpenAIChat {
	if b == nil || b.providers == nil {
		return providers.SecretaryOpenAIChat{}
	}
	active := b.providers.Active()
	id := strings.TrimSpace(active.Secretary)
	if id == "" {
		return providers.SecretaryOpenAIChat{}
	}
	p, ok := b.providers.Get(id)
	if !ok {
		return providers.SecretaryOpenAIChat{}
	}
	return p.Targets.Secretary.OpenAIChat
}

func buildOpenAIChatReceipt(res *http.Response, raw []byte, requestModel string) map[string]any {
	receipt := map[string]any{
		"backend":       "openai-chat",
		"provider":      "openai-compatible",
		"status_code":   0,
		"request_model": strings.TrimSpace(requestModel),
	}
	if res != nil {
		receipt["status_code"] = res.StatusCode
		if rid := firstHeaderValue(res.Header, "x-request-id", "request-id"); rid != "" {
			receipt["request_id"] = rid
		}
	}
	model := strings.TrimSpace(extractTopLevelString(raw, "model"))
	if model == "" {
		model = strings.TrimSpace(requestModel)
	}
	if model != "" {
		receipt["model"] = model
	}
	if usage := extractUsage(raw); len(usage) > 0 {
		receipt["usage"] = usage
	}
	return receipt
}
