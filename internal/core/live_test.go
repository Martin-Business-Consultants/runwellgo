package core

import (
	"encoding/json"
	"testing"
)

// The hub gives each person only what's addressed to them, replays what a
// reconnecting tab missed, and says to read again when it can't.
func TestLiveHub(t *testing.T) {
	var h liveHub
	ann, _, _, _, ok := h.subscribe(1, 0)
	if !ok {
		t.Fatal("subscribe refused")
	}
	h.publish("chat", "message", []int64{1, 2}, json.RawMessage(`{"n":1}`))
	h.publish("chat", "message", []int64{2}, json.RawMessage(`{"n":2}`))
	select {
	case ev := <-ann.ch:
		if ev.id != 1 {
			t.Fatalf("got event %d, want 1", ev.id)
		}
	default:
		t.Fatal("Ann got nothing")
	}
	select {
	case ev := <-ann.ch:
		t.Fatalf("Ann got Bob's event %d", ev.id)
	default:
	}
	if on := h.online([]int64{1, 2}); !on[1] || on[2] {
		t.Fatalf("online = %v, want only 1", on)
	}
	h.unsubscribe(ann)
	if !h.online([]int64{1})[1] {
		t.Fatal("a tab that just left should still count a moment (reconnecting)")
	}

	// Bob reconnects after event 1: he missed 2 (and 1 was his too).
	_, missed, resync, latest, _ := h.subscribe(2, 1)
	if resync || len(missed) != 1 || missed[0].id != 2 || latest != 2 {
		t.Fatalf("missed = %v, resync = %v, latest = %d", missed, resync, latest)
	}

	// Older than what's kept: read again.
	for i := 0; i < liveKept+5; i++ {
		h.publish("chat", "x", []int64{3}, json.RawMessage(`{}`))
	}
	if _, _, resync, _, _ := h.subscribe(3, 1); !resync {
		t.Fatal("a tab that missed more than is kept should resync")
	}

	// A slow tab is cut off rather than holding the rest up.
	slow, _, _, _, _ := h.subscribe(4, 0)
	for i := 0; i < liveBuffer+1; i++ {
		h.publish("chat", "x", []int64{4}, json.RawMessage(`{}`))
	}
	select {
	case <-slow.gone:
	default:
		t.Fatal("a tab that fell behind should be cut off")
	}

	h.close()
	if _, _, _, _, ok := h.subscribe(5, 0); ok {
		t.Fatal("a closed hub opens no streams")
	}
}
