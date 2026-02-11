package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"controlccx/internal/tasks"
)

type mutationResponseBody struct {
	OK      bool           `json:"ok"`
	Action  string         `json:"action"`
	Task    *tasks.Task    `json:"task,omitempty"`
	Queue   map[string]any `json:"queue,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Hint    string         `json:"hint,omitempty"`
}

func decodeMutationResponse(t *testing.T, res *http.Response) mutationResponseBody {
	t.Helper()
	var out mutationResponseBody
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode mutation response: %v", err)
	}
	return out
}

func requireMutationTask(t *testing.T, body mutationResponseBody) tasks.Task {
	t.Helper()
	if !body.OK {
		t.Fatalf("mutation failed: error=%q message=%q details=%v", body.Error, body.Message, body.Details)
	}
	if body.Task == nil {
		t.Fatalf("expected task payload, got body=%+v", body)
	}
	return *body.Task
}

func requireMutationQueue(t *testing.T, body mutationResponseBody) map[string]any {
	t.Helper()
	if !body.OK {
		t.Fatalf("mutation failed: error=%q message=%q details=%v", body.Error, body.Message, body.Details)
	}
	if body.Queue == nil {
		t.Fatalf("expected queue payload, got body=%+v", body)
	}
	return body.Queue
}

func requireMutationProblemCode(t *testing.T, body mutationResponseBody, code string) {
	t.Helper()
	if body.OK {
		t.Fatalf("expected mutation failure, got success: %+v", body)
	}
	if strings.TrimSpace(body.Error) != strings.TrimSpace(code) {
		t.Fatalf("error=%q want %q body=%+v", body.Error, code, body)
	}
}

func requireMutationAction(t *testing.T, body mutationResponseBody, action string) {
	t.Helper()
	if strings.TrimSpace(body.Action) != strings.TrimSpace(action) {
		t.Fatalf("action=%q want %q body=%+v", body.Action, action, body)
	}
}
