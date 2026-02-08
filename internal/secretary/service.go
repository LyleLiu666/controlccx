package secretary

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"controlccx/internal/agentsdk"
	"controlccx/internal/agentsdk/xmlprotocol"
	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/providers"
	"controlccx/internal/secretary/llm"
	"controlccx/internal/tasks"
)

type Service struct {
	cfg       config.Config
	tasks     *tasks.Store
	chat      *chat.Store
	events    *EventStore
	compress  *CompressionStore
	auth      *auth.Store
	providers *providers.Store

	client agentsdk.Client

	sendMu sync.Mutex

	compressOpts CompressionOptions
}

type Option func(*Service)

func WithClient(client agentsdk.Client) Option {
	return func(s *Service) {
		s.client = client
	}
}

func WithEventStore(store *EventStore) Option {
	return func(s *Service) {
		s.events = store
	}
}

func WithCompressionStore(store *CompressionStore) Option {
	return func(s *Service) {
		s.compress = store
	}
}

func WithCompressionOptions(opts CompressionOptions) Option {
	return func(s *Service) {
		s.compressOpts = opts
	}
}

func NewService(cfg config.Config, taskStore *tasks.Store, chatStore *chat.Store, authStore *auth.Store, providersStore *providers.Store, opts ...Option) *Service {
	s := &Service{
		cfg:          cfg,
		tasks:        taskStore,
		chat:         chatStore,
		auth:         authStore,
		providers:    providersStore,
		compressOpts: DefaultCompressionOptions(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	s.compressOpts = normalizeCompressionOptions(s.compressOpts)
	return s
}

func (s *Service) Send(ctx context.Context, userText string) (string, error) {
	if s == nil || s.tasks == nil {
		return "", fmt.Errorf("secretary: tasks store is required")
	}
	if s.chat == nil {
		return "", fmt.Errorf("secretary: chat store is required")
	}
	msg := strings.TrimSpace(userText)
	if msg == "" {
		return "请先输入你的问题。", nil
	}

	// Serialize requests: the secretary chat is currently a single global thread.
	// Without this, concurrent Send() calls can interleave history and confuse the agent.
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	reg := newToolRegistry(s.tasks)

	client := s.client
	if client == nil {
		backend := s.selectBackend()
		client = &llm.Client{Backend: backend}
	}

	var sink agentsdk.EventSink
	var runID string
	if s.events != nil {
		runID = newRunID()
		sink = agentsdk.EventSinkFunc(func(ctx context.Context, ev agentsdk.Event) {
			_ = s.events.Append(ctx, runID, ev)
		})
	}

	promptHistory, err := s.promptHistory(ctx, client, runID)
	if err != nil {
		return "", err
	}

	messages := make([]agentsdk.Message, 0, 1+len(promptHistory)+1)
	messages = append(messages, agentsdk.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, promptHistory...)
	messages = append(messages, agentsdk.Message{Role: "user", Content: msg})

	out, runErr := xmlprotocol.RunLoop(ctx, xmlprotocol.RunLoopInput{
		Client:   client,
		Messages: messages,
		Executor: reg,
		MaxSteps: 60,
		Callbacks: xmlprotocol.Callbacks{
			EventSink: sink,
		},
	})

	reply := strings.TrimSpace(out)
	if runErr != nil {
		reply = strings.TrimSpace(secretaryFailedMessage(s.requestedBackend(), backendNameBestEffort(client), runErr))
	}
	if reply == "" {
		reply = "秘书没有返回内容，请重试。"
	}

	if _, err := s.chat.Append(ctx, chat.RoleUser, msg); err != nil {
		return "", err
	}
	if _, err := s.chat.Append(ctx, chat.RoleAssistant, reply); err != nil {
		return "", err
	}
	_ = s.chat.PruneKeepLast(ctx, 2000)
	if s.events != nil {
		_ = s.events.PruneKeepLastRuns(ctx, 200)
	}
	return reply, nil
}

func (s *Service) History(ctx context.Context, limit int) ([]chat.Message, error) {
	if s == nil || s.chat == nil {
		return nil, fmt.Errorf("secretary: chat store is required")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	return s.chat.Tail(ctx, limit)
}

func (s *Service) Clear(ctx context.Context) error {
	if s == nil || s.chat == nil {
		return fmt.Errorf("secretary: chat store is required")
	}
	return s.chat.Clear(ctx)
}

func (s *Service) requestedBackend() string {
	if s == nil || s.providers == nil {
		return "auto"
	}
	active := s.providers.Active()
	id := strings.TrimSpace(active.Secretary)
	if id == "" {
		return "auto"
	}
	p, ok := s.providers.Get(id)
	if !ok {
		return "auto"
	}
	v := strings.ToLower(strings.TrimSpace(p.Targets.Secretary.Backend))
	if v == "" {
		return "auto"
	}
	return v
}

func (s *Service) selectBackend() llm.Backend {
	mode := s.requestedBackend()

	simple := llm.NewSimpleHTTPBackendWithProviders(s.cfg, s.auth, s.providers)
	claude := llm.NewClaudeCLIBackend(s.cfg, s.auth)
	codex := llm.NewCodexCLIBackend(s.cfg, s.auth)

	switch mode {
	case "simple-http":
		return simple
	case "claude":
		return claude
	case "codex":
		return codex
	default:
		return &llm.AutoBackend{Backends: []llm.Backend{simple, claude, codex}}
	}
}

func backendNameBestEffort(client agentsdk.Client) string {
	if client == nil {
		return ""
	}
	if c, ok := client.(*llm.Client); ok {
		if c != nil && c.Backend != nil {
			return strings.TrimSpace(c.Backend.Name())
		}
	}
	return ""
}

func secretaryFailedMessage(requestedBackend string, provider string, err error) string {
	req := strings.TrimSpace(requestedBackend)
	if req == "" {
		req = "auto"
	}
	name := strings.TrimSpace(provider)
	if name == "" {
		name = "<unknown>"
	}
	detail := ""
	if err != nil {
		detail = truncateRunes(strings.TrimSpace(err.Error()), 800)
	}
	return strings.TrimSpace(fmt.Sprintf(`秘书调用 LLM 失败（backend=%s, provider=%s）：%s

最小排查：
- simple-http：配置 ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN（或 ANTHROPIC_API_KEY）
- claude/codex：确认 CLI 可执行（PATH 或 config.yaml 的 paths.*）
- 也可在 Providers 面板为 secretary 选择 backend`, req, name, detail))
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	b.Grow(max * 4)
	n := 0
	for _, r := range s {
		if n >= max-1 {
			break
		}
		b.WriteRune(r)
		n++
	}
	b.WriteRune('…')
	return b.String()
}

func newRunID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
