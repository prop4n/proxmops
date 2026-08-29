package server

import (
	"bufio"
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
	"time"

	"github.com/prop4n/proxmops/internal/auth"
	"github.com/prop4n/proxmops/internal/crypt"
	"github.com/prop4n/proxmops/internal/settings"
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

	key, err := crypt.LoadOrCreateKey(filepath.Join(t.TempDir(), "key"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		auth:     authSvc,
		status:   status.NewStore(),
		settings: settings.New(st, key),
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

// getJSON performs an authenticated GET against the server.
func getJSON(t *testing.T, srv *Server, cookies []*http.Cookie, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	var body map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
	}
	return rec, body
}

func TestGetSettingsNotConfigured(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)

	rec, body := getJSON(t, srv, cookies, "/api/v1/settings")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body["configured"] != false {
		t.Fatalf("configured = %v, want false", body["configured"])
	}
}

const settingsPayload = `{
	"cluster": {
		"endpoint": "https://pve:8006/api2/json",
		"tokenId": "proxmops@pve!gitops",
		"tokenSecret": "cluster-secret",
		"tokenSecretSet": false,
		"insecureSkipVerify": true
	},
	"source": {
		"repoURL": "https://github.com/me/homelab.git",
		"path": "proxmox",
		"revision": "main",
		"username": "git",
		"token": "git-token",
		"tokenSet": false
	},
	"reconcile": {"intervalSeconds": 30, "autoSync": true, "prune": true, "dryRun": false}
}`

func TestPutThenGetSettingsRoundtrip(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(settingsPayload))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "cluster-secret") || strings.Contains(rec.Body.String(), "git-token") {
		t.Fatal("response leaks secrets")
	}

	_, body := getJSON(t, srv, cookies, "/api/v1/settings")
	if body["configured"] != true {
		t.Fatalf("configured = %v, want true", body["configured"])
	}
	cluster, _ := body["cluster"].(map[string]any)
	if cluster["endpoint"] != "https://pve:8006/api2/json" {
		t.Errorf("endpoint = %v", cluster["endpoint"])
	}
	if cluster["tokenSecretSet"] != true || cluster["tokenSecret"] != "" {
		t.Errorf("cluster secret mask wrong: %v", cluster)
	}
	source, _ := body["source"].(map[string]any)
	if source["tokenSet"] != true || source["token"] != "" {
		t.Errorf("git token mask wrong: %v", source)
	}
	reconcile, _ := body["reconcile"].(map[string]any)
	if reconcile["intervalSeconds"] != float64(30) || reconcile["autoSync"] != true {
		t.Errorf("reconcile wrong: %v", reconcile)
	}
}

func TestPutSettingsKeepsStoredSecretsWhenEmpty(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)
	h := srv.routes()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(settingsPayload))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first put status = %d", rec.Code)
	}

	// Second save without secrets and with a changed endpoint.
	update := `{"cluster":{"endpoint":"https://other:8006/api2/json","tokenId":"id"},"source":{"repoURL":"https://github.com/me/other.git"},"reconcile":{"intervalSeconds":60}}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(update))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second put status = %d; body=%s", rec.Code, rec.Body)
	}

	var body struct {
		Cluster struct {
			TokenSecretSet bool `json:"tokenSecretSet"`
		} `json:"cluster"`
		Source struct {
			TokenSet bool `json:"tokenSet"`
		} `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Cluster.TokenSecretSet || !body.Source.TokenSet {
		t.Error("secrets dropped on save without them")
	}
}

func TestPutSettingsRejectsInvalid(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(`{"reconcile":{"intervalSeconds":30}}`))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSettingsRequireAuth(t *testing.T) {
	srv, _ := testServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/settings"},
		{http.MethodPut, "/api/v1/settings"},
		{http.MethodPost, "/api/v1/settings/test"},
	} {
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestTestSettingsWithoutConfiguration(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/test", strings.NewReader("{}"))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestEventsRequiresAuth(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestEventsStreamsSnapshot connects to the SSE stream, verifies the initial
// snapshot, then verifies that a later store update is pushed without polling.
func TestEventsStreamsSnapshot(t *testing.T) {
	srv, token := testServer(t)
	cookies := authenticatedClient(t, srv, token)
	srv.status.Set(status.Snapshot{
		InSync:    true,
		Resources: []status.Resource{{Kind: "Iso", Name: "debian-12", State: status.StateSynced}},
	})

	ts := httptest.NewServer(srv.routes())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect stream: %v", err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	// scanUntil reads SSE lines until one contains needle or the context dies.
	scanUntil := func(needle string) bool {
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), needle) {
				return true
			}
		}
		return false
	}

	// The initial snapshot carries the pre-existing resource.
	if !scanUntil("debian-12") {
		t.Fatal("initial snapshot not received")
	}

	// A subsequent update must be pushed on the open stream.
	srv.status.Set(status.Snapshot{
		InSync:    false,
		Resources: []status.Resource{{Kind: "VirtualMachine", Name: "web-01", State: status.StateOutOfSync, Action: "update"}},
	})
	if !scanUntil("web-01") {
		t.Fatal("pushed snapshot not received")
	}
}
