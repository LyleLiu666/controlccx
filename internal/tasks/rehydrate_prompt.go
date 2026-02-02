package tasks

import (
	"context"
	"errors"
	"sort"
	"strings"
)

func BuildRehydratePrompt(ctx context.Context, store *Store, conversationID string, nextPrompt string) (string, error) {
	if store == nil {
		return "", errors.New("tasks store not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", errors.New("rehydrate: conversation_id is required")
	}
	nextPrompt = strings.TrimSpace(nextPrompt)
	if nextPrompt == "" {
		nextPrompt = "continue"
	}

	all, err := store.ListTasksWithOptions(ctx, 500, ListTasksOptions{IncludeDeleted: true})
	if err != nil {
		return "", err
	}

	var runs []Task
	for _, t := range all {
		if strings.TrimSpace(t.ConversationID) != conversationID {
			continue
		}
		runs = append(runs, t)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})

	const maxBytes = 60_000
	var (
		segments  []string
		total     int
		truncated bool
	)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		segments = append(segments, s)
		total += len(s)
		for total > maxBytes && len(segments) > 1 {
			truncated = true
			total -= len(segments[0])
			segments = segments[1:]
		}
	}

	for _, run := range runs {
		user := strings.TrimSpace(run.Prompt)
		if user == "" {
			continue
		}
		var assistantParts []string
		logs, err := store.ListLogsFiltered(ctx, run.ID, 0, 2000, ListLogsFilter{Streams: []LogStream{LogAssistant}})
		if err != nil {
			return "", err
		}
		for _, l := range logs {
			if strings.TrimSpace(l.Message) == "" {
				continue
			}
			assistantParts = append(assistantParts, l.Message)
		}
		assistant := strings.TrimSpace(strings.Join(assistantParts, "\n"))

		entry := "[User]\n" + user
		if assistant != "" {
			entry += "\n\n[Assistant]\n" + assistant
		}
		add(entry)
	}

	// Ensure the new instruction is always present at the end.
	add("[User]\n" + nextPrompt)

	headerLines := []string{
		"[controlccx rehydrate]",
		"以下内容由 ControlCCX 从历史 run 的 prompt/输出中拼接生成，可能不完整；已尽量保留最近上下文。",
	}
	if truncated {
		headerLines = append(headerLines, "（提示：上下文过长，已自动截断较早部分。）")
	}
	headerLines = append(headerLines, "[/controlccx rehydrate]", "")

	out := strings.Join(headerLines, "\n") + strings.Join(segments, "\n\n")
	if len(out) > maxBytes {
		out = truncateUTF8(out, maxBytes)
	}
	return out, nil
}

func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := 0
	for i := range s {
		if i > maxBytes {
			break
		}
		cut = i
	}
	if cut <= 0 {
		return ""
	}
	return s[:cut]
}
