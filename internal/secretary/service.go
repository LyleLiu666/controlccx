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
	"controlccx/internal/agentsdk/sessioncompress"
	"controlccx/internal/agentsdk/xmlprotocol"
	"controlccx/internal/auth"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/events"
	"controlccx/internal/providers"
	"controlccx/internal/secretary/llm"
	sectools "controlccx/internal/secretary/tools"
	"controlccx/internal/skills"
	"controlccx/internal/taskops"
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
	skills    *skills.Service
	taskOps   *taskops.Service
	hub       *events.Hub
	fsRoots   []string

	client agentsdk.Client

	sendMu sync.Mutex

	scheduleMu sync.Mutex
	schedules  map[string]*scheduleJob

	eventsPruneMu    sync.Mutex
	eventsLastPrune  time.Time
	eventsSincePrune int

	compressOpts CompressionOptions
}

type Option func(*Service)

// SendHooks exposes best-effort streaming signals from secretary Send.
// All callbacks are optional.
type SendHooks struct {
	// OnVisibleDelta receives visible assistant text chunks as they stream.
	// Returning an error aborts the send.
	OnVisibleDelta func(delta string) error

	// OnTrace receives trace/thinking lines from the tool loop.
	OnTrace func(step int, message string)

	// OnToolCall receives structured tool call events.
	OnToolCall func(step int, event agentsdk.ToolCallEvent)

	// OnToolResult receives structured tool result events.
	OnToolResult func(step int, event agentsdk.ToolResultEvent)

	// OnError receives loop-level errors emitted by the agent runtime.
	OnError func(step int, message string)
}

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

func WithEventHub(hub *events.Hub) Option {
	return func(s *Service) {
		s.hub = hub
	}
}

func WithTaskOps(ops *taskops.Service) Option {
	return func(s *Service) {
		s.taskOps = ops
	}
}

func WithSkills(svc *skills.Service) Option {
	return func(s *Service) {
		s.skills = svc
	}
}

func WithFSRoots(roots []string) Option {
	return func(s *Service) {
		if len(roots) == 0 {
			s.fsRoots = nil
			return
		}
		out := make([]string, 0, len(roots))
		for _, r := range roots {
			v := strings.TrimSpace(r)
			if v == "" {
				continue
			}
			out = append(out, v)
		}
		if len(out) == 0 {
			s.fsRoots = nil
			return
		}
		s.fsRoots = out
	}
}

func NewService(cfg config.Config, taskStore *tasks.Store, chatStore *chat.Store, authStore *auth.Store, providersStore *providers.Store, opts ...Option) *Service {
	s := &Service{
		cfg:          cfg,
		tasks:        taskStore,
		chat:         chatStore,
		auth:         authStore,
		providers:    providersStore,
		schedules:    make(map[string]*scheduleJob),
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
	return s.SendByConversation(ctx, "", userText)
}

func (s *Service) SendByConversation(ctx context.Context, conversationID string, userText string) (string, error) {
	return s.send(ctx, conversationID, userText, nil)
}

func (s *Service) SendStream(ctx context.Context, userText string, hooks *SendHooks) (string, error) {
	return s.SendStreamByConversation(ctx, "", userText, hooks)
}

func (s *Service) SendStreamByConversation(ctx context.Context, conversationID string, userText string, hooks *SendHooks) (string, error) {
	return s.send(ctx, conversationID, userText, hooks)
}

func (s *Service) send(ctx context.Context, conversationID string, userText string, hooks *SendHooks) (string, error) {
	if s == nil || s.tasks == nil {
		return "", fmt.Errorf("secretary: tasks store is required")
	}
	if s.chat == nil {
		return "", fmt.Errorf("secretary: chat store is required")
	}
	conversationID = normalizeConversationID(conversationID)
	msg := strings.TrimSpace(userText)
	if msg == "" {
		return "请先输入你的问题。", nil
	}

	// Serialize requests: the secretary chat is currently a single global thread.
	// Without this, concurrent Send() calls can interleave history and confuse the agent.
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	reg := sectools.NewRegistry(sectools.Deps{
		Tasks:     s.tasks,
		Skills:    s.skills,
		Ops:       s.taskOps,
		Scheduler: s,
		FSRoots:   s.fsRoots,
	})

	client := s.client
	if client == nil {
		backend := llm.NewProviderBackendWithProviders(s.cfg, s.auth, s.providers)
		client = &llm.Client{Backend: backend}
	}

	var sinks []agentsdk.EventSink
	var runID string
	if s.events != nil {
		runID = newRunID()
		sinks = append(sinks, agentsdk.EventSinkFunc(func(ctx context.Context, ev agentsdk.Event) {
			_ = s.events.Append(ctx, runID, ev)
		}))
	}
	if hookSink := hookEventSink(hooks); hookSink != nil {
		sinks = append(sinks, hookSink)
	}
	sink := composeEventSink(sinks...)

	promptHistory, err := s.promptHistory(ctx, client, runID, conversationID)
	if err != nil {
		return "", err
	}

	messages := make([]agentsdk.Message, 0, 1+len(promptHistory)+1)
	messages = append(messages, agentsdk.Message{Role: "system", Content: buildSystemPrompt()})
	messages = append(messages, promptHistory...)
	messages = append(messages, agentsdk.Message{Role: "user", Content: msg})

	stepRecords := make([]xmlprotocol.StepRecord, 0, 8)
	finalAssistantContent := ""
	callbacks := xmlprotocol.Callbacks{
		EventSink: sink,
		ObserveStep: func(record xmlprotocol.StepRecord) {
			stepRecords = append(stepRecords, record)
		},
		ObserveFinal: func(visibleContent, assistantContent string) {
			_ = visibleContent
			finalAssistantContent = strings.TrimSpace(assistantContent)
		},
	}
	if hooks != nil && hooks.OnVisibleDelta != nil {
		callbacks.OnContent = hooks.OnVisibleDelta
	}

	contextCompressThreshold := s.compressOpts.MaxContextRunes
	if contextCompressThreshold < sessioncompress.DefaultMaxContextRunes {
		contextCompressThreshold = sessioncompress.DefaultMaxContextRunes
	}

	out, runErr := xmlprotocol.RunLoop(ctx, xmlprotocol.RunLoopInput{
		Client:                        client,
		Messages:                      messages,
		LLMOptions:                    s.llmOptionsBestEffort(ctx),
		ContextCompressThresholdRunes: contextCompressThreshold,
		Executor:                      reg,
		MaxSteps:                      500,
		Callbacks:                     callbacks,
	})

	reply := strings.TrimSpace(out)
	if runErr != nil {
		reply = strings.TrimSpace(secretaryFailedMessage(backendNameBestEffort(client), runErr))
	}
	if reply == "" {
		reply = "秘书没有返回内容，请重试。"
	}

	if s.events != nil && strings.TrimSpace(runID) != "" {
		if receipt, ok := providerReceiptBestEffort(client); ok && len(receipt) > 0 {
			_ = s.events.Append(ctx, runID, agentsdk.Event{
				Kind:     agentsdk.EventKindProviderReceipt,
				Protocol: "http",
				Step:     0,
				Time:     time.Now().UTC(),
				Payload:  receipt,
			})
		}
	}

	if err := s.appendRunLoopTranscript(ctx, conversationID, msg, stepRecords, finalAssistantContent, reply); err != nil {
		return "", err
	}
	_ = s.chat.PruneKeepLastInConversation(ctx, conversationID, 2000)
	if s.events != nil {
		s.maybePruneEvents(ctx, true)
	}
	return reply, nil
}

func (s *Service) maybePruneEvents(ctx context.Context, force bool) {
	if s == nil || s.events == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	const (
		keepRuns      = 200
		minInterval   = 5 * time.Second
		appendTrigger = 20
	)

	now := time.Now().UTC()
	shouldPrune := force

	s.eventsPruneMu.Lock()
	if !force {
		s.eventsSincePrune++
		if s.eventsSincePrune >= appendTrigger || (!s.eventsLastPrune.IsZero() && now.Sub(s.eventsLastPrune) >= minInterval) {
			shouldPrune = true
		}
	}
	if shouldPrune {
		s.eventsLastPrune = now
		s.eventsSincePrune = 0
	}
	s.eventsPruneMu.Unlock()

	if shouldPrune {
		_ = s.events.PruneKeepLastRuns(ctx, keepRuns)
	}
}

func composeEventSink(sinks ...agentsdk.EventSink) agentsdk.EventSink {
	active := make([]agentsdk.EventSink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			active = append(active, s)
		}
	}
	switch len(active) {
	case 0:
		return nil
	case 1:
		return active[0]
	default:
		return agentsdk.EventSinkFunc(func(ctx context.Context, ev agentsdk.Event) {
			for _, s := range active {
				s.OnEvent(ctx, ev)
			}
		})
	}
}

func hookEventSink(hooks *SendHooks) agentsdk.EventSink {
	if hooks == nil {
		return nil
	}
	if hooks.OnTrace == nil && hooks.OnToolCall == nil && hooks.OnToolResult == nil && hooks.OnError == nil {
		return nil
	}
	return agentsdk.EventSinkFunc(func(ctx context.Context, ev agentsdk.Event) {
		_ = ctx
		switch ev.Kind {
		case agentsdk.EventKindTrace:
			if hooks.OnTrace == nil {
				return
			}
			if payload, ok := ev.Payload.(agentsdk.TraceEvent); ok {
				hooks.OnTrace(ev.Step, strings.TrimSpace(payload.Message))
			}
		case agentsdk.EventKindToolCall:
			if hooks.OnToolCall == nil {
				return
			}
			if payload, ok := ev.Payload.(agentsdk.ToolCallEvent); ok {
				hooks.OnToolCall(ev.Step, payload)
			}
		case agentsdk.EventKindToolResult:
			if hooks.OnToolResult == nil {
				return
			}
			if payload, ok := ev.Payload.(agentsdk.ToolResultEvent); ok {
				hooks.OnToolResult(ev.Step, payload)
			}
		case agentsdk.EventKindError:
			if hooks.OnError == nil {
				return
			}
			if payload, ok := ev.Payload.(agentsdk.ErrorEvent); ok {
				hooks.OnError(ev.Step, strings.TrimSpace(payload.Error))
			}
		}
	})
}

func providerReceiptBestEffort(client agentsdk.Client) (map[string]any, bool) {
	if client == nil {
		return nil, false
	}
	c, ok := client.(*llm.Client)
	if !ok || c == nil || c.Backend == nil {
		return nil, false
	}
	type receiptProvider interface {
		LastReceipt() map[string]any
	}
	p, ok := c.Backend.(receiptProvider)
	if !ok || p == nil {
		return nil, false
	}
	receipt := p.LastReceipt()
	if len(receipt) == 0 {
		return nil, false
	}
	return receipt, true
}

func (s *Service) History(ctx context.Context, limit int) ([]chat.Message, error) {
	return s.HistoryByConversation(ctx, "", limit)
}

func (s *Service) HistoryByConversation(ctx context.Context, conversationID string, limit int) ([]chat.Message, error) {
	if s == nil || s.chat == nil {
		return nil, fmt.Errorf("secretary: chat store is required")
	}
	conversationID = normalizeConversationID(conversationID)
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	const pageSize = 500
	afterID := int64(0)
	filtered := make([]chat.Message, 0, limit)

	for {
		page, err := s.chat.ListInConversation(ctx, conversationID, afterID, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, m := range page {
			afterID = m.ID
			if isInternalTranscriptMessage(m.Role, m.Content) {
				continue
			}
			filtered = append(filtered, m)
			if len(filtered) > limit {
				filtered = filtered[len(filtered)-limit:]
			}
		}
		if len(page) < pageSize {
			break
		}
	}

	if len(filtered) == 0 {
		return nil, nil
	}
	return filtered, nil
}

func (s *Service) Clear(ctx context.Context) error {
	if s == nil || s.chat == nil {
		return fmt.Errorf("secretary: chat store is required")
	}
	return s.chat.Clear(ctx)
}

func (s *Service) ClearByConversation(ctx context.Context, conversationID string) error {
	if s == nil || s.chat == nil {
		return fmt.Errorf("secretary: chat store is required")
	}
	return s.chat.ClearConversation(ctx, conversationID)
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

func secretaryFailedMessage(backend string, err error) string {
	name := strings.TrimSpace(backend)
	if name == "" {
		name = "<unknown>"
	}
	detail := ""
	if err != nil {
		detail = truncateRunes(strings.TrimSpace(err.Error()), 800)
	}
	return strings.TrimSpace(fmt.Sprintf(`秘书调用 LLM 失败（backend=%s）：%s

最小排查：
- 在 Providers 面板为“秘书”选择 backend 并配置对应字段：
  - simple-http：Base URL（可选）、Auth Token（优先）或 API Key、Model（可选）
  - openai-chat：Base URL（可选）、API Key、Model（可选）
- 或直接设置环境变量：
  - simple-http：ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY / ANTHROPIC_MODEL
  - openai-chat：OPENAI_BASE_URL / OPENAI_API_KEY / OPENAI_MODEL
- 如果错误包含 context deadline exceeded：说明单次 LLM 请求超时；可在 config.yaml 里设置 secretary.llm_timeout（例如 30m），或设置 CONTROLCCX_SECRETARY_LLM_TIMEOUT=30m（0 表示不设超时）。`, name, detail))
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

func (s *Service) appendRunLoopTranscript(ctx context.Context, conversationID string, initialUser string, stepRecords []xmlprotocol.StepRecord, finalAssistantContent string, fallbackReply string) error {
	if s == nil || s.chat == nil {
		return fmt.Errorf("secretary: chat store is required")
	}
	if err := s.appendChatMessageIfNonEmpty(ctx, conversationID, chat.RoleUser, initialUser); err != nil {
		return err
	}
	for _, step := range stepRecords {
		if err := s.appendChatMessageIfNonEmpty(ctx, conversationID, chat.RoleAssistant, step.AssistantContent); err != nil {
			return err
		}
		if err := s.appendChatMessageIfNonEmpty(ctx, conversationID, chat.RoleUser, step.ToolResultMessage); err != nil {
			return err
		}
	}

	final := strings.TrimSpace(fallbackReply)
	if final == "" {
		final = strings.TrimSpace(finalAssistantContent)
	}
	return s.appendChatMessageIfNonEmpty(ctx, conversationID, chat.RoleAssistant, final)
}

func (s *Service) appendChatMessageIfNonEmpty(ctx context.Context, conversationID string, role chat.Role, content string) error {
	if s == nil || s.chat == nil {
		return fmt.Errorf("secretary: chat store is required")
	}
	_ = normalizeConversationID(conversationID)
	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}
	_, err := s.chat.AppendInConversation(ctx, conversationID, role, text)
	return err
}

func isInternalTranscriptMessage(role chat.Role, content string) bool {
	text := strings.TrimSpace(content)
	if text == "" {
		return false
	}

	switch role {
	case chat.RoleAssistant:
		return strings.Contains(text, "<tool_data>")
	case chat.RoleUser:
		return strings.Contains(text, "<tool_result>")
	default:
		return false
	}
}

func newRunID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func normalizeConversationID(conversationID string) string {
	return chat.NormalizeConversationID(conversationID)
}

func (s *Service) llmOptionsBestEffort(ctx context.Context) *agentsdk.ChatCompletionOptions {
	opts := &agentsdk.ChatCompletionOptions{
		EnablePromptCache: true,
		CacheEpoch:        1,
	}
	if s == nil || s.compress == nil {
		return opts
	}
	rec, ok, err := s.compress.Latest(ctx)
	if err != nil || !ok || rec.ID <= 0 {
		return opts
	}
	maxInt := int64(^uint(0) >> 1)
	if rec.ID > maxInt {
		opts.CacheEpoch = int(maxInt)
		return opts
	}
	opts.CacheEpoch = int(rec.ID)
	return opts
}
