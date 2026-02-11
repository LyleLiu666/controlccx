package connectors

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestRegisterDevelopmentStubs_Idempotent(t *testing.T) {
	r := NewRegistry()
	if err := RegisterDevelopmentStubs(r, nil); err != nil {
		t.Fatalf("register stubs: %v", err)
	}
	if err := RegisterDevelopmentStubs(r, nil); err != nil {
		t.Fatalf("register stubs second time: %v", err)
	}
	got := r.List()
	if len(got) != 2 {
		t.Fatalf("list=%v, want 2 connectors", got)
	}
	if got[0] != "feishu" || got[1] != "mattermost" {
		t.Fatalf("list=%v, want [feishu mattermost]", got)
	}
}

func TestStubConnector_Send_EchoesNormalizedPayloadAndLogsRoute(t *testing.T) {
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	r := NewRegistry()
	if err := RegisterDevelopmentStubs(r, logger); err != nil {
		t.Fatalf("register stubs: %v", err)
	}

	result, err := r.Send(context.Background(), OutboundMessage{
		Connector:      " MatterMost ",
		ChannelID:      " dev-channel ",
		ConversationID: " conv-1 ",
		ReplyToID:      " msg-1 ",
		Text:           " hello world ",
		Metadata: map[string]string{
			" env ": " local ",
			"":      "ignored",
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if result.Connector != "mattermost" {
		t.Fatalf("result connector=%q, want mattermost", result.Connector)
	}
	if result.Echo.Connector != "mattermost" {
		t.Fatalf("echo connector=%q, want mattermost", result.Echo.Connector)
	}
	if result.Echo.ChannelID != "dev-channel" {
		t.Fatalf("echo channel=%q, want dev-channel", result.Echo.ChannelID)
	}
	if result.Echo.ConversationID != "conv-1" {
		t.Fatalf("echo conversation=%q, want conv-1", result.Echo.ConversationID)
	}
	if result.Echo.ReplyToID != "msg-1" {
		t.Fatalf("echo reply_to=%q, want msg-1", result.Echo.ReplyToID)
	}
	if result.Echo.Text != "hello world" {
		t.Fatalf("echo text=%q, want hello world", result.Echo.Text)
	}
	if got, ok := result.Echo.Metadata["env"]; !ok || got != "local" {
		t.Fatalf("echo metadata=%v, want env=local", result.Echo.Metadata)
	}
	if _, exists := result.Echo.Metadata[""]; exists {
		t.Fatalf("unexpected empty metadata key: %v", result.Echo.Metadata)
	}
	logLine := logBuf.String()
	if !strings.Contains(logLine, "connector=mattermost") {
		t.Fatalf("log=%q, want connector route", logLine)
	}
	if !strings.Contains(logLine, "conversation=conv-1") {
		t.Fatalf("log=%q, want conversation id", logLine)
	}
}
