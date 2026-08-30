package logbuf

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestBufferCapAndSnapshot(t *testing.T) {
	b := New(3)
	log := slog.New(b.Handler(slog.NewTextHandler(io.Discard, nil)))
	for _, m := range []string{"a", "b", "c", "d"} {
		log.Info(m)
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("len = %d, want 3 (capped)", len(snap))
	}
	// Oldest dropped; order preserved.
	if snap[0].Message != "b" || snap[2].Message != "d" {
		t.Fatalf("snapshot = %v, want [b c d]", []string{snap[0].Message, snap[1].Message, snap[2].Message})
	}
}

func TestHandlerCapturesAttrs(t *testing.T) {
	b := New(10)
	log := slog.New(b.Handler(slog.NewTextHandler(io.Discard, nil)))
	log.Info("applied", "action", "create", "vmid", 101)
	snap := b.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("len = %d, want 1", len(snap))
	}
	if snap[0].Level != "INFO" || snap[0].Message != "applied action=create vmid=101" {
		t.Fatalf("entry = %+v", snap[0])
	}
}

func TestSubscribeReceivesNewEntries(t *testing.T) {
	b := New(10)
	log := slog.New(b.Handler(slog.NewTextHandler(io.Discard, nil)))
	ch, cancel := b.Subscribe()
	defer cancel()

	log.Info("live")
	select {
	case e := <-ch:
		if e.Message != "live" {
			t.Fatalf("got %q, want live", e.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the entry")
	}
	_ = context.Background()
}
