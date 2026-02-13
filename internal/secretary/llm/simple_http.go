package llm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"controlccx/internal/agentsdk"
	"controlccx/internal/auth"
	"controlccx/internal/config"
	"controlccx/internal/providers"

	"github.com/goccy/go-json"
)

const (
	defaultSimpleHTTPBaseURL = "https://api.anthropic.com"
	defaultSimpleHTTPModel   = "claude-3-5-sonnet-latest"
	defaultSimpleHTTPTokens  = 2048
)

type SimpleHTTPBackend struct {
	cfg       config.Config
	auth      *auth.Store
	providers *providers.Store
	client    *http.Client

	receiptMu   sync.RWMutex
	lastReceipt map[string]any
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

func (b *SimpleHTTPBackend) LastReceipt() map[string]any {
	if b == nil {
		return nil
	}
	b.receiptMu.RLock()
	defer b.receiptMu.RUnlock()
	return cloneAnyMap(b.lastReceipt)
}

func (b *SimpleHTTPBackend) Complete(ctx context.Context, prompt string) (string, error) {
	return b.CompleteChat(ctx, []agentsdk.Message{{Role: "user", Content: prompt}}, nil)
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicContent struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []anthropicMessage `json:"messages"`
	System        []anthropicContent `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
}

func (b *SimpleHTTPBackend) CompleteChat(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions) (string, error) {
	return b.completeChat(ctx, messages, opts, false, nil)
}

func (b *SimpleHTTPBackend) CompleteChatStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_, err := b.completeChat(ctx, messages, opts, true, callback)
	return err
}

func (b *SimpleHTTPBackend) completeChat(
	ctx context.Context,
	messages []agentsdk.Message,
	opts *agentsdk.ChatCompletionOptions,
	stream bool,
	callback agentsdk.StreamCallback,
) (string, error) {
	b.setLastReceipt(nil)

	timeout := resolveSecretaryLLMTimeout(b.cfg)
	ctx, cancel := withDefaultTimeout(ctx, timeout)
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
	if opts != nil && strings.TrimSpace(opts.Model) != "" {
		model = strings.TrimSpace(opts.Model)
	}

	endpoint, err := normalizeMessagesEndpoint(baseURL)
	if err != nil {
		return "", err
	}

	maxTokens := defaultSimpleHTTPTokens
	if opts != nil && opts.MaxTokens != nil && *opts.MaxTokens > 0 {
		maxTokens = *opts.MaxTokens
	}

	enableCache := true
	if opts != nil {
		enableCache = opts.EnablePromptCache
	}

	systemBlocks, anthropicMessages := buildAnthropicPayload(messages, enableCache)
	cacheMarkedBlocks := countCacheMarkedBlocks(systemBlocks, anthropicMessages)
	if enableCache && opts != nil && opts.CacheEpoch > 0 {
		epochBlock := anthropicContent{
			Type:         "text",
			Text:         fmt.Sprintf("controlccx_kv_epoch=%d", opts.CacheEpoch),
			CacheControl: &anthropicCacheControl{Type: "ephemeral"},
		}
		systemBlocks = append([]anthropicContent{epochBlock}, systemBlocks...)
	}

	reqBody := anthropicRequest{
		Model:         model,
		MaxTokens:     maxTokens,
		Messages:      anthropicMessages,
		System:        systemBlocks,
		Temperature:   nil,
		StopSequences: nil,
		Stream:        stream,
	}
	if opts != nil {
		reqBody.Temperature = opts.Temperature
		if len(opts.Stop) > 0 {
			reqBody.StopSequences = append([]string(nil), opts.Stop...)
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("simple-http marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("simple-http build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	if enableCache {
		req.Header.Set("anthropic-beta", "prompt-caching")
	}
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

	if stream {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
			if readErr != nil {
				return "", fmt.Errorf("simple-http read stream error response: %w", readErr)
			}
			receipt := buildSimpleHTTPReceipt(res, raw, model, enableCache, opts, cacheMarkedBlocks)
			b.setLastReceipt(receipt)

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

		contentType := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Type")))
		if !strings.Contains(contentType, "text/event-stream") {
			raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
			if readErr != nil {
				return "", fmt.Errorf("simple-http read non-stream response: %w", readErr)
			}
			receipt := buildSimpleHTTPReceipt(res, raw, model, enableCache, opts, cacheMarkedBlocks)
			b.setLastReceipt(receipt)

			text := extractCompletionText(raw)
			if strings.TrimSpace(text) == "" {
				return "", fmt.Errorf("simple-http empty completion from %s", endpoint)
			}
			if callback != nil {
				if err := callback(text); err != nil {
					return "", err
				}
			}
			return strings.TrimSpace(text), nil
		}

		text, streamModel, usage, err := parseAnthropicStreamResponse(res.Body, callback)
		receipt := buildSimpleHTTPStreamReceipt(res, model, enableCache, opts, cacheMarkedBlocks, streamModel, usage)
		b.setLastReceipt(receipt)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("simple-http empty completion from %s", endpoint)
		}
		return strings.TrimSpace(text), nil
	}

	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("simple-http read response: %w", err)
	}
	receipt := buildSimpleHTTPReceipt(res, raw, model, enableCache, opts, cacheMarkedBlocks)
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
		return "", fmt.Errorf("simple-http status %d: %s", res.StatusCode, msg)
	}

	text := extractCompletionText(raw)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("simple-http empty completion from %s", endpoint)
	}
	if callback != nil {
		if err := callback(text); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(text), nil
}

func (b *SimpleHTTPBackend) setLastReceipt(receipt map[string]any) {
	if b == nil {
		return
	}
	b.receiptMu.Lock()
	defer b.receiptMu.Unlock()
	b.lastReceipt = cloneAnyMap(receipt)
}

func parseAnthropicStreamResponse(body io.Reader, callback agentsdk.StreamCallback) (string, string, map[string]any, error) {
	if body == nil {
		return "", "", nil, fmt.Errorf("simple-http stream body is nil")
	}
	reader := bufio.NewReader(body)
	var (
		eventName string
		dataLines []string
		text      strings.Builder
		model     string
		usage     map[string]any
	)

	handleEvent := func(name string, dataRaw string) error {
		name = strings.ToLower(strings.TrimSpace(name))
		dataRaw = strings.TrimSpace(dataRaw)
		if dataRaw == "" || dataRaw == "[DONE]" {
			return nil
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(dataRaw), &payload); err != nil {
			return nil
		}
		kind := strings.ToLower(strings.TrimSpace(stringFromAny(payload["type"])))
		if kind == "" {
			kind = name
		}

		if kind == "error" || name == "error" {
			return fmt.Errorf("simple-http stream error: %s", extractStreamErrorMessage(payload))
		}

		switch kind {
		case "message_start":
			msg := mapFromAny(payload["message"])
			if m := strings.TrimSpace(stringFromAny(msg["model"])); m != "" {
				model = m
			}
			usage = mergeUsageFromAny(usage, msg["usage"])
		case "content_block_start":
			block := mapFromAny(payload["content_block"])
			if strings.EqualFold(strings.TrimSpace(stringFromAny(block["type"])), "text") {
				delta := stringFromAny(block["text"])
				if delta != "" {
					text.WriteString(delta)
					if callback != nil {
						if err := callback(delta); err != nil {
							return err
						}
					}
				}
			}
		case "content_block_delta":
			deltaObj := mapFromAny(payload["delta"])
			deltaType := strings.ToLower(strings.TrimSpace(stringFromAny(deltaObj["type"])))
			if deltaType != "" && deltaType != "text_delta" {
				return nil
			}
			delta := stringFromAny(deltaObj["text"])
			if delta == "" {
				return nil
			}
			text.WriteString(delta)
			if callback != nil {
				if err := callback(delta); err != nil {
					return err
				}
			}
		case "message_delta":
			usage = mergeUsageFromAny(usage, payload["usage"])
		}
		return nil
	}

	flush := func() error {
		if len(dataLines) == 0 && strings.TrimSpace(eventName) == "" {
			return nil
		}
		dataRaw := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		name := eventName
		eventName = ""
		return handleEvent(name, dataRaw)
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", "", nil, fmt.Errorf("simple-http read stream: %w", err)
		}
		line = strings.TrimRight(line, "\n")
		line = strings.TrimRight(line, "\r")

		if strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return "", "", nil, err
			}
		} else {
			switch {
			case strings.HasPrefix(line, ":"):
				// SSE comment/heartbeat
			case strings.HasPrefix(line, "event:"):
				eventName = strings.TrimSpace(line[len("event:"):])
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}
	if err := flush(); err != nil {
		return "", "", nil, err
	}

	return strings.TrimSpace(text.String()), strings.TrimSpace(model), cloneAnyMap(usage), nil
}

func extractStreamErrorMessage(payload map[string]any) string {
	if len(payload) == 0 {
		return "unknown error"
	}
	if errObj := mapFromAny(payload["error"]); len(errObj) > 0 {
		if msg := strings.TrimSpace(stringFromAny(errObj["message"])); msg != "" {
			return msg
		}
	}
	if msg := strings.TrimSpace(stringFromAny(payload["message"])); msg != "" {
		return msg
	}
	return "unknown error"
}

func mergeUsageFromAny(dst map[string]any, v any) map[string]any {
	src := mapFromAny(v)
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]any{}
	}
	for k, vv := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if child := mapFromAny(vv); len(child) > 0 {
			merged := map[string]any{}
			if prev := mapFromAny(dst[key]); len(prev) > 0 {
				for ck, cv := range prev {
					merged[ck] = cv
				}
			}
			for ck, cv := range child {
				merged[ck] = cv
			}
			dst[key] = merged
			continue
		}
		dst[key] = vv
	}
	return dst
}

func mapFromAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	if len(m) == 0 {
		return nil
	}
	return m
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func buildAnthropicPayload(messages []agentsdk.Message, enableCache bool) ([]anthropicContent, []anthropicMessage) {
	cacheIndexes := map[int]bool{}
	if enableCache {
		cacheIndexes = cacheMessageIndexes(messages)
	}

	system := make([]anthropicContent, 0)
	converted := make([]anthropicMessage, 0)

	for idx, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}

		block := anthropicContent{
			Type: "text",
			Text: content,
		}
		if enableCache && cacheIndexes[idx] {
			block.CacheControl = &anthropicCacheControl{Type: "ephemeral"}
		}

		if role == "system" {
			system = append(system, block)
			continue
		}
		if role != "user" && role != "assistant" {
			role = "user"
		}

		converted = append(converted, anthropicMessage{
			Role:    role,
			Content: []anthropicContent{block},
		})
	}

	return system, converted
}

func cacheMessageIndexes(messages []agentsdk.Message) map[int]bool {
	indexes := make(map[int]bool)
	systemCount := 0

	for i, msg := range messages {
		if strings.ToLower(strings.TrimSpace(msg.Role)) == "system" {
			indexes[i] = true
			systemCount++
			if systemCount >= 2 {
				break
			}
		}
	}

	for i := len(messages) - 1; i >= 0 && i >= len(messages)-2; i-- {
		indexes[i] = true
	}

	return indexes
}

func countCacheMarkedBlocks(system []anthropicContent, messages []anthropicMessage) int {
	count := 0
	for _, blk := range system {
		if blk.CacheControl != nil && strings.TrimSpace(blk.CacheControl.Type) != "" {
			count++
		}
	}
	for _, msg := range messages {
		for _, blk := range msg.Content {
			if blk.CacheControl != nil && strings.TrimSpace(blk.CacheControl.Type) != "" {
				count++
			}
		}
	}
	return count
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

func buildSimpleHTTPReceipt(
	res *http.Response,
	raw []byte,
	requestModel string,
	cacheEnabled bool,
	opts *agentsdk.ChatCompletionOptions,
	cacheMarkedBlocks int,
) map[string]any {
	receipt := buildSimpleHTTPBaseReceipt(res, requestModel, cacheEnabled, opts, cacheMarkedBlocks)

	model := strings.TrimSpace(extractTopLevelString(raw, "model"))
	if model == "" {
		model = strings.TrimSpace(requestModel)
	}
	if model != "" {
		receipt["model"] = model
	}

	if usage := extractUsage(raw); len(usage) > 0 {
		receipt["usage"] = usage
		if kv := deriveKVCacheUsage(usage); len(kv) > 0 {
			receipt["kv_cache"] = kv
		}
	}
	return receipt
}

func buildSimpleHTTPStreamReceipt(
	res *http.Response,
	requestModel string,
	cacheEnabled bool,
	opts *agentsdk.ChatCompletionOptions,
	cacheMarkedBlocks int,
	responseModel string,
	usage map[string]any,
) map[string]any {
	receipt := buildSimpleHTTPBaseReceipt(res, requestModel, cacheEnabled, opts, cacheMarkedBlocks)
	model := strings.TrimSpace(responseModel)
	if model == "" {
		model = strings.TrimSpace(requestModel)
	}
	if model != "" {
		receipt["model"] = model
	}
	if len(usage) > 0 {
		usageCopy := cloneAnyMap(usage)
		receipt["usage"] = usageCopy
		if kv := deriveKVCacheUsage(usageCopy); len(kv) > 0 {
			receipt["kv_cache"] = kv
		}
	}
	return receipt
}

func buildSimpleHTTPBaseReceipt(
	res *http.Response,
	requestModel string,
	cacheEnabled bool,
	opts *agentsdk.ChatCompletionOptions,
	cacheMarkedBlocks int,
) map[string]any {
	receipt := map[string]any{
		"backend":                      "simple-http",
		"provider":                     "anthropic-compatible",
		"status_code":                  0,
		"request_model":                strings.TrimSpace(requestModel),
		"request_prompt_cache_enabled": cacheEnabled,
		"request_cache_marked_blocks":  cacheMarkedBlocks,
	}
	if opts != nil && opts.CacheEpoch > 0 {
		receipt["request_cache_epoch"] = opts.CacheEpoch
	}
	if res != nil {
		receipt["status_code"] = res.StatusCode
		if rid := firstHeaderValue(res.Header, "request-id", "x-request-id", "anthropic-request-id"); rid != "" {
			receipt["request_id"] = rid
		}
	}
	return receipt
}

func extractTopLevelString(raw []byte, key string) string {
	if len(raw) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return ""
	}
	v, _ := obj[key]
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func extractUsage(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil
	}
	usageAny, ok := obj["usage"]
	if !ok {
		return nil
	}
	usage, _ := usageAny.(map[string]any)
	if len(usage) == 0 {
		return nil
	}
	return cloneAnyMap(usage)
}

func deriveKVCacheUsage(usage map[string]any) map[string]any {
	if len(usage) == 0 {
		return nil
	}
	kv := map[string]any{}

	if n, ok := toInt64(usage["cache_read_input_tokens"]); ok {
		kv["cache_read_input_tokens"] = n
	}
	if n, ok := toInt64(usage["cache_creation_input_tokens"]); ok {
		kv["cache_creation_input_tokens"] = n
	}
	if n, ok := toInt64(usage["cached_input_tokens"]); ok {
		kv["cached_input_tokens"] = n
	}

	if promptDetails, _ := usage["prompt_tokens_details"].(map[string]any); len(promptDetails) > 0 {
		if n, ok := toInt64(promptDetails["cached_tokens"]); ok {
			kv["prompt_cached_tokens"] = n
		}
	}
	if len(kv) == 0 {
		return nil
	}
	return kv
}

func toInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int64(x), true
	case float32:
		return int64(x), true
	case float64:
		return int64(x), true
	default:
		return 0, false
	}
}

func firstHeaderValue(h http.Header, names ...string) string {
	for _, name := range names {
		v := strings.TrimSpace(h.Get(name))
		if v != "" {
			return v
		}
	}
	return ""
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out[k] = cloneAnyMap(x)
		case []any:
			out[k] = cloneAnySlice(x)
		default:
			out[k] = x
		}
	}
	return out
}

func cloneAnySlice(in []any) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, 0, len(in))
	for _, v := range in {
		switch x := v.(type) {
		case map[string]any:
			out = append(out, cloneAnyMap(x))
		case []any:
			out = append(out, cloneAnySlice(x))
		default:
			out = append(out, x)
		}
	}
	return out
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
