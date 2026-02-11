package connectors

import (
	"context"
	"fmt"
	"time"
)

type RouteLogger interface {
	Printf(format string, v ...any)
}

type stubConnector struct {
	name   string
	logger RouteLogger
	now    func() time.Time
}

func NewMattermostStub(logger RouteLogger) Connector {
	return &stubConnector{name: "mattermost", logger: logger, now: time.Now}
}

func NewFeishuStub(logger RouteLogger) Connector {
	return &stubConnector{name: "feishu", logger: logger, now: time.Now}
}

func RegisterDevelopmentStubs(r *Registry, logger RouteLogger) error {
	if r == nil {
		return ErrInvalidConnector
	}
	for _, c := range []Connector{
		NewMattermostStub(logger),
		NewFeishuStub(logger),
	} {
		if _, exists := r.Get(c.Name()); exists {
			continue
		}
		if err := r.Register(c); err != nil {
			return err
		}
	}
	return nil
}

func (s *stubConnector) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

func (s *stubConnector) Send(_ context.Context, msg OutboundMessage) (DeliveryResult, error) {
	if s == nil || normalizeName(s.name) == "" {
		return DeliveryResult{}, ErrInvalidConnector
	}
	msg = normalizeOutboundMessage(msg)
	// Route is determined by the stub itself; keep connector consistent in echo payload.
	msg.Connector = s.name
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	if s.logger != nil {
		s.logger.Printf(
			"connector_stub route connector=%s channel=%s conversation=%s reply_to=%s",
			s.name,
			msg.ChannelID,
			msg.ConversationID,
			msg.ReplyToID,
		)
	}
	return DeliveryResult{
		Connector: s.name,
		MessageID: fmt.Sprintf("stub-%s-%d", s.name, now.UnixNano()),
		SentAt:    now,
		Echo:      msg,
	}, nil
}
