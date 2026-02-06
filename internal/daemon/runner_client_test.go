package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunnerClient_StartAndCancel(t *testing.T) {
	var started string
	var canceled string
	const token = "tok"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(InstanceTokenHeader); got != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/api/runner/tasks/t1/start":
			started = "t1"
			w.WriteHeader(http.StatusOK)
		case "/api/runner/tasks/t1/cancel":
			canceled = "t1"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewRunnerClient(srv.URL, RunnerClientOptions{Token: token})
	if err != nil {
		t.Fatalf("NewRunnerClient: %v", err)
	}

	if err := c.Start(context.Background(), "t1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started != "t1" {
		t.Fatalf("started=%q want %q", started, "t1")
	}

	ok, err := c.Cancel(context.Background(), "t1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !ok {
		t.Fatalf("Cancel ok=false want true")
	}
	if canceled != "t1" {
		t.Fatalf("canceled=%q want %q", canceled, "t1")
	}
}
