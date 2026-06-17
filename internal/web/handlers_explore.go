package web

import (
	"net/http"
	"strconv"
	"time"

	"recipe-s3-exporter/internal/auth"
)

func (s *Server) explore(w http.ResponseWriter, r *http.Request) {
	targets, _ := s.db.ListTargets(r.Context())
	vm := ExploreVM{
		User:    auth.UserFrom(r.Context()),
		Flash:   s.flash(r),
		Targets: targets,
	}

	if idStr := r.URL.Query().Get("target_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil && id > 0 {
			vm.Selected = id
			store, err := s.storeForTarget(r.Context(), id)
			if err != nil {
				vm.ListErr = "Cannot open target: " + err.Error()
			} else if objs, err := store.List(r.Context(), ""); err != nil {
				vm.ListErr = "Cannot list objects: " + err.Error()
			} else {
				vm.Objects = objs
			}
		}
	}
	s.render(w, r, ExplorePage(vm))
}

// exploreDownload redirects to a short-lived presigned URL for an object.
func (s *Server) exploreDownload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("target_id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	store, err := s.storeForTarget(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	url, err := store.PresignGet(r.Context(), key, 5*time.Minute)
	if err != nil {
		http.Error(w, "presign failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}
