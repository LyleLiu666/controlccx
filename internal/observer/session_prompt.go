package observer

import (
	"context"
	"sort"
	"strings"

	"controlccx/internal/tasks"
)

func looksLikeContinuePrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return true
	}
	if p == "continue" || p == "继续" || p == "继续执行" || p == "继续完成" {
		return true
	}
	if p == "resume" || p == "retry" {
		return true
	}
	return false
}

// bestEffortSessionPrompt returns a stable, user-facing "base prompt" for the session.
// It prefers the first ModeNew prompt in the same session; for resume runs with a generic
// "continue" prompt, it falls back to the best candidate in session history.
func (s *Service) bestEffortSessionPrompt(ctx context.Context, t tasks.Task) string {
	p := strings.TrimSpace(t.Prompt)
	sid := strings.TrimSpace(t.SessionID)
	if sid == "" {
		return p
	}
	if p != "" && !looksLikeContinuePrompt(p) {
		return p
	}
	if s == nil || s.Store == nil {
		return p
	}

	all, err := s.Store.ListTasks(ctx, 500)
	if err != nil || len(all) == 0 {
		return p
	}

	var sess []tasks.Task
	for _, it := range all {
		if strings.TrimSpace(it.SessionID) != sid {
			continue
		}
		if strings.TrimSpace(it.Prompt) == "" {
			continue
		}
		sess = append(sess, it)
	}
	if len(sess) == 0 {
		return p
	}
	sort.SliceStable(sess, func(i, j int) bool { return sess[i].CreatedAt.Before(sess[j].CreatedAt) })

	// Prefer the earliest ModeNew prompt that isn't just "continue".
	for _, it := range sess {
		if it.Mode != tasks.ModeNew {
			continue
		}
		pp := strings.TrimSpace(it.Prompt)
		if pp == "" || looksLikeContinuePrompt(pp) {
			continue
		}
		return pp
	}

	// Otherwise: pick the longest prompt in the session.
	best := strings.TrimSpace(sess[0].Prompt)
	for _, it := range sess {
		pp := strings.TrimSpace(it.Prompt)
		if len(pp) > len(best) && !looksLikeContinuePrompt(pp) {
			best = pp
		}
	}
	return strings.TrimSpace(best)
}
