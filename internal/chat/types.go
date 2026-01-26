package chat

import "time"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	Role    Role      `json:"role"`
	Content string    `json:"content"`
}

