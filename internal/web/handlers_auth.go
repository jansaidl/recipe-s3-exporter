package web

import (
	"net/http"

	"recipe-s3-exporter/internal/auth"
)

func (s *Server) getLogin(w http.ResponseWriter, r *http.Request) {
	if auth.UserFrom(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, r, LoginPage(""))
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	if _, err := s.auth.Login(r.Context(), email, password); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		s.render(w, r, LoginPage("Invalid email or password."))
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	_ = s.auth.Logout(r.Context())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jobs, _ := s.db.ListJobs(ctx)
	targets, _ := s.db.ListTargets(ctx)
	tokens, _ := s.db.ListTokens(ctx)
	active, _ := s.db.ListActiveRuns(ctx)
	recent, _ := s.db.ListRecentRuns(ctx, 15)

	s.render(w, r, Dashboard(DashboardVM{
		User:       auth.UserFrom(ctx),
		Flash:      s.flash(r),
		Jobs:       len(jobs),
		Targets:    len(targets),
		Tokens:     len(tokens),
		ActiveRuns: active,
		Recent:     recent,
	}))
}
