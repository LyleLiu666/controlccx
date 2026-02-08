package events

import (
	"testing"
	"time"
)

func TestHub_Publish_AssignsMonotonicSeq(t *testing.T) {
	hub := NewHub()
	ch, unsub := hub.Subscribe(8)
	t.Cleanup(unsub)

	hub.Publish(Event{Type: "a", Time: time.Now().UTC()})
	hub.Publish(Event{Type: "b", Time: time.Now().UTC()})

	e1 := <-ch
	e2 := <-ch

	if e1.Seq <= 0 {
		t.Fatalf("seq1=%d, want >0", e1.Seq)
	}
	if e2.Seq != e1.Seq+1 {
		t.Fatalf("seq2=%d, want %d", e2.Seq, e1.Seq+1)
	}
	if got := hub.Cursor(); got != e2.Seq {
		t.Fatalf("cursor=%d, want %d", got, e2.Seq)
	}
}
