package connectors

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeConnector struct {
	name     string
	received []OutboundMessage
}

func (f *fakeConnector) Name() string { return f.name }

func (f *fakeConnector) Send(_ context.Context, msg OutboundMessage) (DeliveryResult, error) {
	f.received = append(f.received, msg)
	return DeliveryResult{Connector: f.name, MessageID: "m-1", SentAt: time.Unix(0, 0).UTC()}, nil
}

func TestRegistry_RegisterAndList(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeConnector{name: "mattermost"}); err != nil {
		t.Fatalf("register mattermost: %v", err)
	}
	if err := r.Register(&fakeConnector{name: "feishu"}); err != nil {
		t.Fatalf("register feishu: %v", err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("len(list)=%d, want 2", len(list))
	}
	if list[0] != "feishu" || list[1] != "mattermost" {
		t.Fatalf("list=%v, want sorted [feishu mattermost]", list)
	}
}

func TestRegistry_RejectDuplicateAndEmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&fakeConnector{name: "mattermost"}); err != nil {
		t.Fatalf("register mattermost: %v", err)
	}
	if err := r.Register(&fakeConnector{name: "mattermost"}); !errors.Is(err, ErrConnectorExists) {
		t.Fatalf("duplicate err=%v, want ErrConnectorExists", err)
	}
	if err := r.Register(&fakeConnector{name: ""}); !errors.Is(err, ErrInvalidConnector) {
		t.Fatalf("empty-name err=%v, want ErrInvalidConnector", err)
	}
}

func TestRegistry_Send_NormalizesConnectorName(t *testing.T) {
	r := NewRegistry()
	c := &fakeConnector{name: "mattermost"}
	if err := r.Register(c); err != nil {
		t.Fatalf("register connector: %v", err)
	}
	_, err := r.Send(context.Background(), OutboundMessage{
		Connector: " MatterMost ",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(c.received) != 1 {
		t.Fatalf("received=%d, want 1", len(c.received))
	}
	if c.received[0].Connector != "mattermost" {
		t.Fatalf("normalized connector=%q, want %q", c.received[0].Connector, "mattermost")
	}
}
