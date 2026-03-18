package chat

import (
	"strings"
	"time"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

const GlobalConversationID = "__global__"

func NormalizeConversationID(conversationID string) string {
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return GlobalConversationID
	}
	return id
}

type Message struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	Role    Role      `json:"role"`
	Content string    `json:"content"`
}
