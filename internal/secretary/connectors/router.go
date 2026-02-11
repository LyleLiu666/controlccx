package connectors

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrMessageHandlerMissing = errors.New("connectors: message handler not configured")

type Router struct {
	handler MessageHandler
	audit   AuditHook
}

func NewRouter(handler MessageHandler, audit AuditHook) *Router {
	return &Router{handler: handler, audit: audit}
}

func (r *Router) Receive(ctx context.Context, msg InboundMessage) error {
	if r == nil || r.handler == nil {
		return ErrMessageHandlerMissing
	}
	now := time.Now().UTC()
	msg.Connector = normalizeName(msg.Connector)
	if msg.Connector == "" {
		return ErrInvalidConnector
	}
	msg.MessageID = strings.TrimSpace(msg.MessageID)
	if msg.ReceivedAt.IsZero() {
		msg.ReceivedAt = now
	}

	r.record(ctx, ConnectorEvent{
		Connector: msg.Connector,
		MessageID: msg.MessageID,
		Stage:     EventStageReceived,
		At:        now,
	})

	if err := r.handler.HandleConnectorMessage(ctx, msg); err != nil {
		r.record(ctx, ConnectorEvent{
			Connector: msg.Connector,
			MessageID: msg.MessageID,
			Stage:     EventStageFailed,
			Reason:    err.Error(),
			At:        time.Now().UTC(),
		})
		return err
	}

	r.record(ctx, ConnectorEvent{
		Connector: msg.Connector,
		MessageID: msg.MessageID,
		Stage:     EventStageHandled,
		At:        time.Now().UTC(),
	})
	return nil
}

func (r *Router) record(ctx context.Context, evt ConnectorEvent) {
	if r == nil || r.audit == nil {
		return
	}
	_ = r.audit.RecordConnectorEvent(ctx, evt)
}
