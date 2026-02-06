package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ServeSSE returns an SSE handler that streams events from the hub.
//
// The stream includes:
// - an initial "hello" event
// - periodic "heartbeat" events
// - any events published to the hub
func ServeSSE(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if hub == nil {
			http.Error(w, "events not available", http.StatusServiceUnavailable)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		eventsCh, unsubscribe := hub.Subscribe(256)
		defer unsubscribe()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		send := func(evt Event) {
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		}

		send(Event{Type: "hello", Time: time.Now().UTC(), Payload: map[string]any{"ok": true}})

		for {
			select {
			case <-r.Context().Done():
				return
			case evt := <-eventsCh:
				send(evt)
			case <-heartbeat.C:
				send(Event{Type: "heartbeat", Time: time.Now().UTC()})
			}
		}
	}
}
