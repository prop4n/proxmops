package store

import (
	"context"
	"fmt"
	"time"
)

// ResourceEvent is one recorded change in a resource's history.
type ResourceEvent struct {
	ID     int64     `json:"id"`
	Kind   string    `json:"kind"`
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Reason string    `json:"reason,omitempty"`
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
}

// AppendEvent records one resource event.
func (s *Store) AppendEvent(ctx context.Context, e ResourceEvent) error {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO resource_events (kind, name, type, reason, commit_, at) VALUES (?, ?, ?, ?, ?, ?)",
		e.Kind, e.Name, e.Type, e.Reason, e.Commit, at)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// EventsFor returns a resource's events, newest first, capped at limit.
func (s *Store) EventsFor(ctx context.Context, kind, name string, limit int) ([]ResourceEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, kind, name, type, reason, commit_, at FROM resource_events WHERE kind = ? AND name = ? ORDER BY id DESC LIMIT ?",
		kind, name, limit)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []ResourceEvent
	for rows.Next() {
		var e ResourceEvent
		if err := rows.Scan(&e.ID, &e.Kind, &e.Name, &e.Type, &e.Reason, &e.Commit, &e.At); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
