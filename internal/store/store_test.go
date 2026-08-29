package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// open returns a Store backed by a fresh temporary database.
func open(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAccountsCRUD(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if n, err := st.CountAccounts(ctx); err != nil || n != 0 {
		t.Fatalf("CountAccounts on empty store = %d, %v; want 0, nil", n, err)
	}

	created, err := st.CreateAccount(ctx, "alice", "hash-a")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if created.ID == 0 || created.Username != "alice" || created.PasswordHash != "hash-a" {
		t.Fatalf("CreateAccount returned %+v", created)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreateAccount left CreatedAt zero")
	}

	if n, err := st.CountAccounts(ctx); err != nil || n != 1 {
		t.Fatalf("CountAccounts after create = %d, %v; want 1, nil", n, err)
	}

	byName, err := st.AccountByUsername(ctx, "alice")
	if err != nil || byName.ID != created.ID {
		t.Fatalf("AccountByUsername = %+v, %v", byName, err)
	}
	byID, err := st.AccountByID(ctx, created.ID)
	if err != nil || byID.Username != "alice" {
		t.Fatalf("AccountByID = %+v, %v", byID, err)
	}
}

func TestAccountUsernameIsUnique(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if _, err := st.CreateAccount(ctx, "bob", "h1"); err != nil {
		t.Fatalf("first CreateAccount: %v", err)
	}
	if _, err := st.CreateAccount(ctx, "bob", "h2"); err == nil {
		t.Fatal("second CreateAccount with same username: want error, got nil")
	}
}

func TestAccountNotFound(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if _, err := st.AccountByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AccountByID(missing) error = %v; want ErrNotFound", err)
	}
	if _, err := st.AccountByUsername(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AccountByUsername(missing) error = %v; want ErrNotFound", err)
	}
}

func TestSessionsLifecycle(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	acct, err := st.CreateAccount(ctx, "carol", "h")
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := st.CreateSession(ctx, "hash-1", acct.ID, exp); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := st.Session(ctx, "hash-1")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got.AccountID != acct.ID || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("Session = %+v; want account %d exp %v", got, acct.ID, exp)
	}

	if err := st.DeleteSession(ctx, "hash-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := st.Session(ctx, "hash-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Session after delete = %v; want ErrNotFound", err)
	}
}

func TestDeleteExpiredSessionsKeepsLive(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	acct, _ := st.CreateAccount(ctx, "dave", "h")
	now := time.Now().UTC().Truncate(time.Second)
	if err := st.CreateSession(ctx, "expired", acct.ID, now.Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if err := st.CreateSession(ctx, "live", acct.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession live: %v", err)
	}

	if err := st.DeleteExpiredSessions(ctx, now); err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}

	if _, err := st.Session(ctx, "expired"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session survived: %v", err)
	}
	if _, err := st.Session(ctx, "live"); err != nil {
		t.Errorf("live session removed: %v", err)
	}
}

// Exercises the ON DELETE CASCADE, which needs the foreign_keys pragma.
func TestSessionCascadeOnAccountDelete(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	acct, _ := st.CreateAccount(ctx, "erin", "h")
	if err := st.CreateSession(ctx, "sess", acct.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if _, err := st.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", acct.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if _, err := st.Session(ctx, "sess"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session survived account delete: %v; want ErrNotFound (cascade)", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	st := open(t)
	ctx := context.Background()

	if _, err := st.LoadSettings(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadSettings on empty store = %v; want ErrNotFound", err)
	}

	if err := st.SaveSettings(ctx, []byte("blob-1")); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	data, err := st.LoadSettings(ctx)
	if err != nil || string(data) != "blob-1" {
		t.Fatalf("LoadSettings = %q, %v; want \"blob-1\"", data, err)
	}

	// A second save overwrites the single row rather than inserting.
	if err := st.SaveSettings(ctx, []byte("blob-2")); err != nil {
		t.Fatalf("SaveSettings overwrite: %v", err)
	}
	data, err = st.LoadSettings(ctx)
	if err != nil || string(data) != "blob-2" {
		t.Fatalf("LoadSettings after overwrite = %q, %v; want \"blob-2\"", data, err)
	}

	var count int
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings").Scan(&count); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if count != 1 {
		t.Fatalf("settings row count = %d; want 1 (single-row upsert)", count)
	}
}
