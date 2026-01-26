package events

import (
	"sync"
	"time"
)

type Event struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Payload any       `json:"payload,omitempty"`
}

type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan Event]struct{})}
}

func (h *Hub) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan Event, buffer)

	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}

	return ch, unsub
}

func (h *Hub) Publish(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- evt:
		default:
			// Drop events if subscriber is too slow; UI can refetch state/logs via APIs.
		}
	}
}

