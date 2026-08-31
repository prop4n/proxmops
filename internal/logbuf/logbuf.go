// Package logbuf keeps the daemon's most recent log lines in memory and fans
// new ones out to subscribers, so the web UI can show a live tail. It wraps an
// existing slog.Handler rather than replacing it: lines still go to stdout.
package logbuf

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Entry is one captured log line.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Buffer is a bounded ring of recent log entries with live subscribers. It is
// safe for concurrent use.
type Buffer struct {
	mu      sync.Mutex
	entries []Entry
	cap     int

	subsMu sync.Mutex
	subs   map[chan Entry]struct{}
}

// New returns a Buffer retaining up to capacity entries.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Buffer{
		entries: make([]Entry, 0, capacity),
		cap:     capacity,
		subs:    make(map[chan Entry]struct{}),
	}
}

// Snapshot returns a copy of the retained entries, oldest first.
func (b *Buffer) Snapshot() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Subscribe registers a listener for new entries. The channel is buffered; a
// slow consumer drops entries rather than blocking the daemon. The returned
// func unregisters it.
func (b *Buffer) Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, 64)
	b.subsMu.Lock()
	b.subs[ch] = struct{}{}
	b.subsMu.Unlock()
	return ch, func() {
		b.subsMu.Lock()
		delete(b.subs, ch)
		b.subsMu.Unlock()
	}
}

// add appends an entry, dropping the oldest past capacity, and broadcasts it.
func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	if len(b.entries) == b.cap {
		b.entries = append(b.entries[:0], b.entries[1:]...)
	}
	b.entries = append(b.entries, e)
	b.mu.Unlock()

	b.subsMu.Lock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // drop for a slow consumer
		}
	}
	b.subsMu.Unlock()
}

// Handler wraps inner so every record is also captured in the buffer.
func (b *Buffer) Handler(inner slog.Handler) slog.Handler {
	return &handler{inner: inner, buf: b}
}

// handler tees slog records into the buffer, then delegates to inner.
type handler struct {
	inner slog.Handler
	buf   *Buffer
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&sb, " %s=%v", a.Key, a.Value)
		return true
	})
	h.buf.add(Entry{Time: r.Time, Level: r.Level.String(), Message: sb.String()})
	return h.inner.Handle(ctx, r)
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &handler{inner: h.inner.WithAttrs(as), buf: h.buf}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{inner: h.inner.WithGroup(name), buf: h.buf}
}
