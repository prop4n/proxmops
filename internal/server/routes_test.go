package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/status"
	"github.com/prop4n/proxmops/internal/store"
)

// testServer builds a Server backed by a temp store and returns it plus the
// one-time setup token logged by auth.Init.
func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	var logBuf bytes.Buffer
	authSvc := auth.New(st, slog.New(slog.NewTextHandler(&logBuf, nil)))
	if err := authSvc.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, after, _ := strings.Cut(logBuf.String(), "setupToken=")
	token := strings.Fields(after)[0]

	srv := &Server{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		auth:   authSvc,
		status: status.NewStore(),
	}
	return srv, token
}

// authenticatedClient completes setup and returns the session cookies.
func authenticatedClient(t *testing.T, srv *Server, token string) []*http.Cookie {
	t.Helper()
	body := `{"token":"` + token + `","username":"admin","password":"pw"}`
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup failed: %d", rec.Code)
	}
	return rec.Result().Cookies()
}

func TestResourcesRequiresAuth(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/resources", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestResourcesReturnsSnapshot(t *testing.T) {
	srv, token := testServer(t)
	srv.status.Set(status.Snapshot{
		InSync:    false,
		Resources: []status.Resource{{Kind: "Iso", Name: "debian-12", State: status.StateOutOfSync, Action: "create"}},
	})
	cookies := authenticatedClient(t, srv, token)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "debian-12") || !strings.Contains(rec.Body.String(), "OutOfSync") {
		t.Fatalf("snapshot not returned: %s", rec.Body)
	}
}

func TestHealthEndpointPublic(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSetupThenAuthenticatedMe(t *testing.T) {
	srv, token := testServer(t)
	h := srv.routes()

	// Setup status should report that setup is needed.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil))
	var status struct{ NeedsSetup bool }
	json.NewDecoder(rec.Body).Decode(&status)
	if !status.NeedsSetup {
		t.Fatal("expected needsSetup=true")
	}

	// Complete setup.
	body := `{"token":"` + token + `","username":"admin","password":"pw"}`
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, want 201; body=%s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("setup should set a session cookie")
	}

	// Use the session cookie to reach /me.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "admin") {
		t.Fatalf("me body = %s, want admin", rec.Body)
	}
}

func TestSetupRejectsBadToken(t *testing.T) {
	srv, _ := testServer(t)
	body := `{"token":"nope","username":"admin","password":"pw"}`
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
