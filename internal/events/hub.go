package events

import (
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Seq     int64     `json:"seq,omitempty"`
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
	Payload any       `json:"payload,omitempty"`
}

type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
	seq  atomic.Int64
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan Event]struct{})}
}

func (h *Hub) Cursor() int64 {
	if h == nil {
		return 0
	}
	return h.seq.Load()
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
	evt.Seq = h.seq.Add(1)
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
