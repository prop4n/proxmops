package auth

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prop4n/proxmops/internal/store"
)

// newService returns a Service backed by a temp SQLite store and a buffer
// logger, plus a function to extract the setup token from the log.
func newService(t *testing.T) (*Service, func() string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	var buf bytes.Buffer
	svc := New(st, slog.New(slog.NewTextHandler(&buf, nil)))
	extract := func() string {
		_, after, ok := strings.Cut(buf.String(), "setupToken=")
		if !ok {
			return ""
		}
		return strings.Fields(after)[0]
	}
	return svc, extract
}

func TestSetupFlow(t *testing.T) {
	svc, token := newService(t)
	ctx := context.Background()

	needs, _ := svc.NeedsSetup(ctx)
	if !needs {
		t.Fatal("fresh store should need setup")
	}
	if err := svc.Init(ctx); err != nil {
		t.Fatal(err)
	}
	setupToken := token()
	if setupToken == "" {
		t.Fatal("Init should log a setup token")
	}

	if _, err := svc.Setup(ctx, "wrong-token", "admin", "pw"); err != ErrInvalidBootstrap {
		t.Fatalf("wrong token: got %v, want ErrInvalidBootstrap", err)
	}

	session, err := svc.Setup(ctx, setupToken, "admin", "pw")
	if err != nil || session == "" {
		t.Fatalf("setup: session=%q err=%v", session, err)
	}

	needs, _ = svc.NeedsSetup(ctx)
	if needs {
		t.Fatal("setup should be complete now")
	}
	if _, err := svc.Setup(ctx, setupToken, "admin2", "pw"); err != ErrSetupClosed {
		t.Fatalf("second setup: got %v, want ErrSetupClosed", err)
	}
}

func TestLoginAndSession(t *testing.T) {
	svc, token := newService(t)
	ctx := context.Background()
	_ = svc.Init(ctx)
	if _, err := svc.Setup(ctx, token(), "admin", "pw"); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Login(ctx, "admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("bad password: got %v, want ErrInvalidCredentials", err)
	}

	session, err := svc.Login(ctx, "admin", "pw")
	if err != nil {
		t.Fatal(err)
	}
	account, err := svc.Authenticate(ctx, session)
	if err != nil || account.Username != "admin" {
		t.Fatalf("authenticate: account=%+v err=%v", account, err)
	}

	if err := svc.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, session); err != ErrInvalidCredentials {
		t.Fatalf("after logout: got %v, want ErrInvalidCredentials", err)
	}
}
