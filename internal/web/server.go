// Package web wires the HTTP server, routes, and request handlers for the
// dashboard.
package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/config"
	"recipe-s3-exporter/internal/crypto"
	"recipe-s3-exporter/internal/db"
	"recipe-s3-exporter/internal/scheduler"
	"recipe-s3-exporter/internal/worker"
)

//go:embed static
var staticFS embed.FS

// Server holds shared dependencies for all handlers.
type Server struct {
	cfg    *config.Config
	db     *db.DB
	cipher *crypto.Cipher
	auth   *auth.Service
	pool   *worker.Pool
	sched  *scheduler.Scheduler
}

// NewServer constructs the web server.
func NewServer(cfg *config.Config, store *db.DB, cipher *crypto.Cipher, a *auth.Service, pool *worker.Pool, sched *scheduler.Scheduler) *Server {
	return &Server{cfg: cfg, db: store, cipher: cipher, auth: a, pool: pool, sched: sched}
}

// Routes builds the chi router with the full middleware stack.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(s.auth.SM.LoadAndSave)
	r.Use(s.auth.LoadUser)

	sub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	// Public routes.
	r.Get("/login", s.getLogin)
	r.Post("/login", s.postLogin)
	r.Post("/logout", s.postLogout)

	// Authenticated routes.
	r.Group(func(r chi.Router) {
		r.Use(s.auth.RequireAuth)

		r.Get("/", s.dashboard)
		r.Get("/jobs", s.listJobs)
		r.Post("/jobs/{id}/run", s.runJob)
		r.Get("/runs", s.listRuns)
		r.Get("/partials/active-runs", s.activeRunsPartial)
		r.Get("/explore", s.explore)
		r.Get("/explore/download", s.exploreDownload)

		// Admin-only routes.
		r.Group(func(r chi.Router) {
			r.Use(s.auth.RequireAdmin)

			r.Get("/jobs/new", s.newJob)
			r.Post("/jobs", s.createJob)
			r.Get("/jobs/{id}/edit", s.editJob)
			r.Post("/jobs/{id}", s.updateJob)
			r.Post("/jobs/{id}/toggle", s.toggleJob)
			r.Post("/jobs/{id}/delete", s.deleteJob)

			r.Get("/partials/projects", s.projectsPartial)
			r.Get("/partials/services", s.servicesPartial)
			r.Get("/partials/backup-tags", s.backupTagsPartial)

			r.Get("/tokens", s.listTokens)
			r.Post("/tokens", s.createToken)
			r.Post("/tokens/{id}/validate", s.validateToken)
			r.Post("/tokens/{id}/delete", s.deleteToken)

			r.Get("/targets", s.listTargets)
			r.Post("/targets", s.createTarget)
			r.Post("/targets/{id}/test", s.testTarget)
			r.Post("/targets/{id}/delete", s.deleteTarget)

			r.Get("/users", s.listUsers)
			r.Post("/users", s.createUser)
			r.Post("/users/{id}/role", s.setUserRole)
			r.Post("/users/{id}/delete", s.deleteUser)
		})
	})

	return r
}

// render writes a templ component as the HTTP response.
func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("[web] render: %v", err)
	}
}

// flash pops the one-shot flash message for this request.
func (s *Server) flash(r *http.Request) string {
	return s.auth.SM.PopString(r.Context(), "flash")
}

// setFlash stores a flash message for the next request.
func (s *Server) setFlash(r *http.Request, msg string) {
	s.auth.SM.Put(r.Context(), "flash", msg)
}
