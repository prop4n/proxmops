package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/prop4n/proxmops/web"
)

// routes builds the HTTP handler: a JSON API under /api/v1 plus the embedded UI.
func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(s.requestLogger)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)

		// Public auth endpoints.
		r.Get("/setup", s.handleSetupStatus)
		r.Post("/setup", s.handleSetup)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/me", s.handleMe)
			r.Get("/resources", s.handleResources)
			r.Delete("/resources/{kind}/{name}", s.handleDeleteResource)
			r.Get("/resources/{kind}/{name}/events", s.handleResourceEvents)
			r.Get("/events", s.handleEvents)
			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Post("/settings/test", s.handleTestSettings)
		})
	})

	r.Handle("/*", spaHandler(web.Assets()))
	return r
}

// handleHealth reports daemon liveness.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleResources reports the managed resources and their sync status from the
// most recent reconciliation pass.
func (s *Server) handleResources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.status.Get())
}

// requestLogger logs each request through the server's structured logger.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"reqID", middleware.GetReqID(r.Context()),
		)
	})
}
