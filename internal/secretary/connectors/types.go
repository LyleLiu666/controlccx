package connectors

import (
	"context"
	"strings"
	"time"
)

type InboundMessage struct {
	Connector      string            `json:"connector"`
	ChannelID      string            `json:"channel_id,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
	MessageID      string            `json:"message_id,omitempty"`
	UserID         string            `json:"user_id,omitempty"`
	Text           string            `json:"text"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	ReceivedAt     time.Time         `json:"received_at,omitempty"`
}

type OutboundMessage struct {
	Connector      string            `json:"connector"`
	ChannelID      string            `json:"channel_id,omitempty"`
	ConversationID string            `json:"conversation_id,omitempty"`
	ReplyToID      string            `json:"reply_to_id,omitempty"`
	Text           string            `json:"text"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type DeliveryResult struct {
	Connector string    `json:"connector"`
	MessageID string    `json:"message_id,omitempty"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

type Connector interface {
	Name() string
	Send(ctx context.Context, msg OutboundMessage) (DeliveryResult, error)
}

type MessageHandler interface {
	HandleConnectorMessage(ctx context.Context, msg InboundMessage) error
}

type EventStage string

const (
	EventStageReceived EventStage = "received"
	EventStageHandled  EventStage = "handled"
	EventStageFailed   EventStage = "failed"
)

type ConnectorEvent struct {
	Connector string     `json:"connector"`
	MessageID string     `json:"message_id,omitempty"`
	Stage     EventStage `json:"stage"`
	Reason    string     `json:"reason,omitempty"`
	At        time.Time  `json:"at"`
}

type AuditHook interface {
	RecordConnectorEvent(ctx context.Context, evt ConnectorEvent) error
}

func normalizeName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
