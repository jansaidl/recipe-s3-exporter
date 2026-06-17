package web

import (
	"log"
	"net/http"
	"sort"
	"strconv"
)

func (s *Server) projectsPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		s.render(w, r, InlineError("Project", "Choose a token first."))
		return
	}
	log.Printf("[partials] projects: token_id=%d", tokenID)

	tok, err := s.db.GetToken(r.Context(), tokenID)
	if err != nil {
		log.Printf("[partials] projects: load token %d failed: %v", tokenID, err)
		s.render(w, r, InlineError("Project", "Token not found."))
		return
	}
	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		log.Printf("[partials] projects: build client failed: %v", err)
		s.render(w, r, InlineError("Project", "Cannot read token: "+err.Error()))
		return
	}
	clientID := tok.ClientID
	if clientID == "" {
		if clientID, err = zc.GetClientID(r.Context()); err != nil {
			log.Printf("[partials] projects: resolve client id failed: %v", err)
			s.render(w, r, InlineError("Project", "Could not resolve account from token: "+err.Error()))
			return
		}
	}
	projects, err := zc.ListProjects(r.Context(), clientID)
	if err != nil {
		log.Printf("[partials] projects: list (client=%s) failed: %v", clientID, err)
		s.render(w, r, InlineError("Project", "Could not list projects: "+err.Error()))
		return
	}
	log.Printf("[partials] projects: client=%s returned %d project(s)", clientID, len(projects))
	s.render(w, r, ProjectSelect(projects))
}

func (s *Server) servicesPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		s.render(w, r, InlineError("Service", "Choose a token first."))
		return
	}
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		s.render(w, r, InlineError("Service", "Choose a project first."))
		return
	}
	log.Printf("[partials] services: token_id=%d project_id=%s", tokenID, projectID)

	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		log.Printf("[partials] services: build client failed: %v", err)
		s.render(w, r, InlineError("Service", "Cannot read token: "+err.Error()))
		return
	}
	services, err := zc.ListServices(r.Context(), projectID)
	if err != nil {
		log.Printf("[partials] services: list (project=%s) failed: %v", projectID, err)
		s.render(w, r, InlineError("Service", "Could not list services: "+err.Error()))
		return
	}
	log.Printf("[partials] services: project=%s returned %d service(s)", projectID, len(services))
	s.render(w, r, ServiceSelect(services))
}

func (s *Server) backupTagsPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		s.render(w, r, InlineError("Backup tag filter", "Choose a token first."))
		return
	}
	serviceID := r.URL.Query().Get("service_stack_id")
	if serviceID == "" {
		s.render(w, r, InlineError("Backup tag filter", "Choose a service first."))
		return
	}
	log.Printf("[partials] backup-tags: token_id=%d service=%s", tokenID, serviceID)

	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		log.Printf("[partials] backup-tags: build client failed: %v", err)
		s.render(w, r, InlineError("Backup tag filter", "Cannot read token: "+err.Error()))
		return
	}
	backups, err := zc.ListBackups(r.Context(), serviceID)
	if err != nil {
		log.Printf("[partials] backup-tags: list backups (service=%s) failed: %v", serviceID, err)
		s.render(w, r, InlineError("Backup tag filter", "Could not list backups: "+err.Error()))
		return
	}
	set := map[string]struct{}{}
	for _, b := range backups {
		for _, t := range b.Tags {
			set[t] = struct{}{}
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	log.Printf("[partials] backup-tags: service=%s has %d backup(s), %d distinct tag(s)", serviceID, len(backups), len(tags))
	s.render(w, r, TagPicker(tags))
}
