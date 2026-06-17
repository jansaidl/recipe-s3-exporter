package web

import (
	"net/http"

	"recipe-s3-exporter/internal/auth"
)

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	active, _ := s.db.ListActiveRuns(r.Context())
	recent, _ := s.db.ListRecentRuns(r.Context(), 100)
	s.render(w, r, RunsPage(RunsVM{
		User:   auth.UserFrom(r.Context()),
		Flash:  s.flash(r),
		Active: active,
		Recent: recent,
	}))
}

// activeRunsPartial powers the self-refreshing progress panel.
func (s *Server) activeRunsPartial(w http.ResponseWriter, r *http.Request) {
	active, _ := s.db.ListActiveRuns(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.render(w, r, ActiveRuns(active))
}
