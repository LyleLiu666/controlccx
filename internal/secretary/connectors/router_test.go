package connectors

import (
	"context"
	"errors"
	"testing"
)

type fakeHandler struct {
	calls int
	last  InboundMessage
	err   error
}

func (f *fakeHandler) HandleConnectorMessage(_ context.Context, msg InboundMessage) error {
	f.calls++
	f.last = msg
	return f.err
}

type fakeAudit struct {
	events []ConnectorEvent
	err    error
}

func (f *fakeAudit) RecordConnectorEvent(_ context.Context, evt ConnectorEvent) error {
	f.events = append(f.events, evt)
	return f.err
}

func TestRouter_Receive_PublishesAuditAndCallsHandler(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAudit{}
	r := NewRouter(h, a)

	msg := InboundMessage{
		Connector:      "mattermost",
		ChannelID:      "c-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		UserID:         "u-1",
		Text:           "hello",
	}
	if err := r.Receive(context.Background(), msg); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d, want 1", h.calls)
	}
	if h.last.MessageID != "msg-1" {
		t.Fatalf("handler last=%q, want %q", h.last.MessageID, "msg-1")
	}
	if len(a.events) != 2 {
		t.Fatalf("audit events=%d, want 2", len(a.events))
	}
	if a.events[0].Stage != EventStageReceived {
		t.Fatalf("first stage=%q, want %q", a.events[0].Stage, EventStageReceived)
	}
	if a.events[1].Stage != EventStageHandled {
		t.Fatalf("second stage=%q, want %q", a.events[1].Stage, EventStageHandled)
	}
}

func TestRouter_Receive_ReportsHandlerFailure(t *testing.T) {
	h := &fakeHandler{err: errors.New("handler failed")}
	r := NewRouter(h, nil)
	err := r.Receive(context.Background(), InboundMessage{Connector: "feishu", MessageID: "m-1"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRouter_Receive_RequiresHandler(t *testing.T) {
	r := NewRouter(nil, nil)
	err := r.Receive(context.Background(), InboundMessage{Connector: "feishu", MessageID: "m-1"})
	if !errors.Is(err, ErrMessageHandlerMissing) {
		t.Fatalf("err=%v, want ErrMessageHandlerMissing", err)
	}
}

func TestRouter_Receive_RejectsEmptyConnector(t *testing.T) {
	h := &fakeHandler{}
	r := NewRouter(h, nil)
	err := r.Receive(context.Background(), InboundMessage{Connector: "  ", MessageID: "m-1"})
	if !errors.Is(err, ErrInvalidConnector) {
		t.Fatalf("err=%v, want ErrInvalidConnector", err)
	}
	if h.calls != 0 {
		t.Fatalf("handler calls=%d, want 0", h.calls)
	}
}
