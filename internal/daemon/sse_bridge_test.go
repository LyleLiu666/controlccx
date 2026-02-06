package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"controlccx/internal/events"
)

func TestBridgeSSEToHub_RepublishesEvents(t *testing.T) {
	remoteHub := events.NewHub()
	dstHub := events.NewHub()
	const token = "tok"

	mux := http.NewServeMux()
	mux.Handle("/api/events", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(InstanceTokenHeader); got != token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		events.ServeSSE(remoteHub)(w, r)
	}))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = BridgeSSEToHub(ctx, srv.URL+"/api/events", dstHub, SSEBridgeOptions{Token: token})
	}()

	ch, unsub := dstHub.Subscribe(8)
	defer unsub()

	// Wait for the bridge to connect (remote SSE sends a "hello" immediately after subscribe).
	connected := false
	connectDeadline := time.NewTimer(2 * time.Second)
	for !connected {
		select {
		case <-connectDeadline.C:
			t.Fatalf("timeout waiting for bridge connect")
		case evt := <-ch:
			if evt.Type == "hello" {
				connected = true
			}
		}
	}
	connectDeadline.Stop()

	remoteHub.Publish(events.Event{Type: "task.updated", Time: time.Now().UTC(), Payload: map[string]any{"id": "t1"}})

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			t.Fatalf("timeout waiting for bridged event")
		case evt := <-ch:
			if evt.Type == "task.updated" {
				return
			}
		}
	}
}
