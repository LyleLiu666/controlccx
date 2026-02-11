package tasks

import "strings"

// ConversationAnchorForTask returns the canonical conversation-scoped key
// ("c:<conversation_id>") used as the stable anchor across runs.
//
// Legacy tasks without conversation_id return empty; callers may fallback
// to SessionKey(taskID, sessionID) when needed.
func ConversationAnchorForTask(t Task) string {
	cid := strings.TrimSpace(t.ConversationID)
	if cid == "" {
		return ""
	}
	return ConversationKey(cid)
}

