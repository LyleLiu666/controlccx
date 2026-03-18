package secretary

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"controlccx/internal/agentsdk"
	"controlccx/internal/agentsdk/sessioncompress"
	"controlccx/internal/chat"
)

type CompressionOptions struct {
	MaxContextRunes        int
	KeepTextMessages       int
	MaxSummaryRunes        int
	MaxMsgRunesForHistory  int
	MaxMessagesToSummarize int
	MaxCompressionSteps    int
	SummaryMaxTokens       int
}

func DefaultCompressionOptions() CompressionOptions {
	return CompressionOptions{
		MaxContextRunes:        sessioncompress.DefaultMaxContextRunes,
		KeepTextMessages:       40,
		MaxSummaryRunes:        8000,
		MaxMsgRunesForHistory:  2000,
		MaxMessagesToSummarize: 500,
		MaxCompressionSteps:    2,
		SummaryMaxTokens:       512,
	}
}

func normalizeCompressionOptions(opts CompressionOptions) CompressionOptions {
	def := DefaultCompressionOptions()
	if opts.MaxContextRunes <= 0 {
		opts.MaxContextRunes = def.MaxContextRunes
	}
	if opts.KeepTextMessages <= 0 {
		opts.KeepTextMessages = def.KeepTextMessages
	}
	if opts.KeepTextMessages > 500 {
		opts.KeepTextMessages = 500
	}
	if opts.MaxSummaryRunes <= 0 {
		opts.MaxSummaryRunes = def.MaxSummaryRunes
	}
	if opts.MaxMsgRunesForHistory <= 0 {
		opts.MaxMsgRunesForHistory = def.MaxMsgRunesForHistory
	}
	if opts.MaxMessagesToSummarize <= 0 {
		opts.MaxMessagesToSummarize = def.MaxMessagesToSummarize
	}
	if opts.MaxMessagesToSummarize > 500 {
		opts.MaxMessagesToSummarize = 500
	}
	if opts.MaxCompressionSteps <= 0 {
		opts.MaxCompressionSteps = def.MaxCompressionSteps
	}
	if opts.SummaryMaxTokens <= 0 {
		opts.SummaryMaxTokens = def.SummaryMaxTokens
	}
	return opts
}

func (s *Service) promptHistory(ctx context.Context, client agentsdk.Client, runID string, conversationID string) ([]agentsdk.Message, error) {
	if s == nil || s.chat == nil {
		return nil, fmt.Errorf("secretary: chat store is required")
	}
	_ = normalizeConversationID(conversationID)
	if s.compress == nil {
		history, err := s.chat.Tail(ctx, 40)
		if err != nil {
			return nil, err
		}
		return chatMessagesToPromptHistory(history, s.compressOpts.MaxMsgRunesForHistory), nil
	}

	rec, ok, err := s.compress.Latest(ctx)
	if err != nil {
		return nil, err
	}
	var (
		cursor  int64
		summary string
	)
	if ok {
		cursor = rec.CursorAfter
		summary = rec.Summary
	}

	summary, cursor, err = s.maybeCompress(ctx, client, runID, summary, cursor)
	if err != nil {
		return nil, err
	}

	// Build prompt from: rolling summary (optional) + unsummarized tail after cursor.
	out := make([]agentsdk.Message, 0, 64)
	if strings.TrimSpace(summary) != "" {
		out = append(out, agentsdk.Message{Role: "system", Content: formatSummaryForPrompt(summary)})
	}

	list, err := s.chat.TailAfter(ctx, cursor, s.compressOpts.KeepTextMessages)
	if err != nil {
		return nil, err
	}
	out = append(out, chatMessagesToPromptHistory(list, s.compressOpts.MaxMsgRunesForHistory)...)
	return out, nil
}

func chatMessagesToPromptHistory(history []chat.Message, maxMsgRunes int) []agentsdk.Message {
	out := make([]agentsdk.Message, 0, len(history))
	for _, m := range history {
		role := strings.TrimSpace(string(m.Role))
		if role == "" {
			role = "user"
		}
		content := truncateRunes(strings.TrimSpace(m.Content), maxMsgRunes)
		if content == "" {
			continue
		}
		out = append(out, agentsdk.Message{Role: role, Content: content})
	}
	return out
}

func formatSummaryForPrompt(summary string) string {
	s := strings.TrimSpace(summary)
	if s == "" {
		return ""
	}
	return strings.TrimSpace("以下是系统自动生成的对话摘要（用于节省上下文，不一定逐字准确，仅供参考）：\n\n" + s)
}

func (s *Service) maybeCompress(ctx context.Context, client agentsdk.Client, runID string, summary string, cursor int64) (string, int64, error) {
	if s == nil || s.chat == nil {
		return "", 0, fmt.Errorf("secretary: chat store is required")
	}
	if s.compress == nil {
		return truncateRunes(summary, s.compressOpts.MaxSummaryRunes), cursor, nil
	}

	cur := cursor
	if cur < 0 {
		cur = 0
	}
	sum := truncateRunes(summary, s.compressOpts.MaxSummaryRunes)

	for i := 0; i < s.compressOpts.MaxCompressionSteps; i++ {
		tail, err := s.chat.TailAfter(ctx, cur, s.compressOpts.KeepTextMessages)
		if err != nil {
			return sum, cur, err
		}
		if len(tail) == 0 {
			break
		}
		keepFrom := tail[0].ID

		// Summarize only messages strictly before the kept tail window.
		head, err := s.chat.List(ctx, cur, s.compressOpts.MaxMessagesToSummarize)
		if err != nil {
			return sum, cur, err
		}
		toSummarize := make([]chat.Message, 0, len(head))
		for _, m := range head {
			if m.ID >= keepFrom {
				break
			}
			toSummarize = append(toSummarize, m)
		}
		if len(toSummarize) == 0 {
			break
		}

		mapped := make([]sessioncompress.Message, 0, len(toSummarize))
		for _, m := range toSummarize {
			id := uint(m.ID)
			mapped = append(mapped, sessioncompress.Message{
				ID:      id,
				Role:    strings.TrimSpace(string(m.Role)),
				Type:    "text",
				Content: truncateRunes(strings.TrimSpace(m.Content), 4000),
			})
		}

		deltaTranscript := sessioncompress.FormatForSummaryInput(mapped, sessioncompress.DefaultOptions())
		if strings.TrimSpace(deltaTranscript) == "" {
			break
		}

		backend := backendNameBestEffort(client)
		newSummary, sumErr := s.summarizeSecretaryHistory(ctx, client, runID, sum, deltaTranscript, s.compressOpts.SummaryMaxTokens)
		if sumErr != nil {
			// Append an error record for observability; keep current summary/cursor.
			_, _ = s.compress.Append(ctx, CompressionRecord{
				CursorBefore: cur,
				CursorAfter:  cur,
				KeepFrom:     keepFrom,
				Summary:      sum,
				Backend:      backend,
				Error:        sumErr.Error(),
			})
			break
		}

		newSummary = truncateRunes(newSummary, s.compressOpts.MaxSummaryRunes)
		if strings.TrimSpace(newSummary) == "" {
			_, _ = s.compress.Append(ctx, CompressionRecord{
				CursorBefore: cur,
				CursorAfter:  cur,
				KeepFrom:     keepFrom,
				Summary:      sum,
				Backend:      backend,
				Error:        "compression summary was empty",
			})
			break
		}

		cursorAfter := toSummarize[len(toSummarize)-1].ID

		_, err = s.compress.Append(ctx, CompressionRecord{
			CursorBefore: cur,
			CursorAfter:  cursorAfter,
			KeepFrom:     keepFrom,
			Summary:      newSummary,
			Backend:      backend,
			Error:        "",
		})
		if err != nil {
			return sum, cur, err
		}

		sum = newSummary
		cur = cursorAfter
	}

	return sum, cur, nil
}

func (s *Service) summarizeSecretaryHistory(ctx context.Context, client agentsdk.Client, runID string, prevSummary string, deltaTranscript string, maxTokens int) (string, error) {
	if client == nil {
		return "", errors.New("missing llm client")
	}
	prev := strings.TrimSpace(prevSummary)
	delta := strings.TrimSpace(deltaTranscript)
	if delta == "" {
		return prev, nil
	}

	system := strings.TrimSpace(`你是一个对话摘要器。你的任务是把“已有摘要”与“新增对话片段”合并成一个新的摘要。

要求：
- 输出必须是纯中文文本（不要 Markdown、不要代码块、不要 XML 标签）
- 保留对话中的关键事实、约束、决定、结论、待办
- 尽量简洁，避免冗余
- 如果已有摘要为空，就基于新增片段生成摘要`)

	user := strings.TrimSpace(fmt.Sprintf(`已有摘要：
%s

新增对话片段：
%s

请输出更新后的摘要：`, prev, delta))

	opts := &agentsdk.ChatCompletionOptions{
		MaxTokens:         &maxTokens,
		EnablePromptCache: true,
		CacheEpoch:        1,
	}

	messages := []agentsdk.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}

	if s != nil && s.events != nil && strings.TrimSpace(runID) != "" {
		_ = s.events.Append(ctx, runID, agentsdk.Event{
			Kind:     agentsdk.EventKindLLMRequest,
			Protocol: "compress",
			Step:     0,
			Time:     time.Now(),
			Payload: agentsdk.LLMRequestEvent{
				Messages: append([]agentsdk.Message(nil), messages...),
				Options:  opts,
			},
		})
	}

	out, err := completeText(ctx, client, messages, opts)
	if s != nil && s.events != nil && strings.TrimSpace(runID) != "" {
		_ = s.events.Append(ctx, runID, agentsdk.Event{
			Kind:     agentsdk.EventKindLLMResponse,
			Protocol: "compress",
			Step:     0,
			Time:     time.Now(),
			Payload: agentsdk.LLMResponseEvent{
				Raw:     out,
				Visible: out,
				Error:   errString(err),
			},
		})
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
