package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"controlccx/internal/agentsdk"
	"controlccx/internal/chat"
	"controlccx/internal/config"
	"controlccx/internal/db"
	"controlccx/internal/secretary"
	"controlccx/internal/tasks"
)

type scriptedClient struct {
	responses []string
	i         int
}

func (c *scriptedClient) ChatCompletionStream(ctx context.Context, messages []agentsdk.Message, opts *agentsdk.ChatCompletionOptions, callback agentsdk.StreamCallback) error {
	_ = ctx
	_ = messages
	_ = opts
	if c.i >= len(c.responses) {
		return callback("no more scripted responses")
	}
	out := c.responses[c.i]
	c.i++
	return callback(out)
}

func TestAPI_SecretaryEndpoints(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, db.Options{Path: filepath.Join(t.TempDir(), "controlccx.db")})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	taskStore := tasks.NewStore(conn)
	chatStore := chat.NewStore(conn)

	llmClient := &scriptedClient{
		responses: []string{
			"<tool_data><call><tool_name>tasks_count</tool_name></call></tool_data>",
			"ok",
		},
	}
	sec := secretary.NewService(config.Default(), taskStore, chatStore, nil, nil, secretary.WithClient(llmClient))

	apiSvc := &API{
		Tasks:     taskStore,
		Secretary: sec,
	}
	srv := httptest.NewServer(apiSvc.Handler())
	t.Cleanup(srv.Close)

	// POST message -> reply
	body, _ := json.Marshal(map[string]any{"message": "hi"})
	res, err := http.Post(srv.URL+"/api/secretary/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", res.StatusCode, http.StatusOK)
	}
	var out struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.TrimSpace(out.Reply) != "ok" {
		t.Fatalf("reply=%q want %q", out.Reply, "ok")
	}

	// GET history
	getRes, err := http.Get(srv.URL + "/api/secretary/messages?limit=10")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	t.Cleanup(func() { _ = getRes.Body.Close() })
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", getRes.StatusCode, http.StatusOK)
	}
	var hist struct {
		Messages []chat.Message `json:"messages"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist.Messages) != 2 {
		t.Fatalf("messages len=%d want 2", len(hist.Messages))
	}
	if hist.Messages[0].Role != chat.RoleUser || strings.TrimSpace(hist.Messages[0].Content) != "hi" {
		t.Fatalf("unexpected user message: %+v", hist.Messages[0])
	}
	if hist.Messages[1].Role != chat.RoleAssistant || strings.TrimSpace(hist.Messages[1].Content) != "ok" {
		t.Fatalf("unexpected assistant message: %+v", hist.Messages[1])
	}

	// Clear
	clearRes, err := http.Post(srv.URL+"/api/secretary/clear", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("post clear: %v", err)
	}
	t.Cleanup(func() { _ = clearRes.Body.Close() })
	if clearRes.StatusCode != http.StatusOK {
		t.Fatalf("clear status=%d want %d", clearRes.StatusCode, http.StatusOK)
	}

	getRes2, err := http.Get(srv.URL + "/api/secretary/messages?limit=10")
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	t.Cleanup(func() { _ = getRes2.Body.Close() })
	if getRes2.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want %d", getRes2.StatusCode, http.StatusOK)
	}
	var hist2 struct {
		Messages []chat.Message `json:"messages"`
	}
	if err := json.NewDecoder(getRes2.Body).Decode(&hist2); err != nil {
		t.Fatalf("decode history after clear: %v", err)
	}
	if len(hist2.Messages) != 0 {
		t.Fatalf("messages len=%d want 0", len(hist2.Messages))
	}
}
