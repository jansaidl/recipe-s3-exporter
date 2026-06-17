package web

import (
	"net/http"
	"sort"
	"strconv"
)

func (s *Server) projectsPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		http.Error(w, "choose a token", http.StatusBadRequest)
		return
	}
	tok, err := s.db.GetToken(r.Context(), tokenID)
	if err != nil {
		http.Error(w, "token not found", http.StatusBadRequest)
		return
	}
	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	clientID := tok.ClientID
	if clientID == "" {
		if clientID, err = zc.GetClientID(r.Context()); err != nil {
			http.Error(w, "resolve client: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	projects, err := zc.ListProjects(r.Context(), clientID)
	if err != nil {
		http.Error(w, "list projects: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.render(w, r, ProjectSelect(projects))
}

func (s *Server) servicesPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		http.Error(w, "choose a token", http.StatusBadRequest)
		return
	}
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "choose a project", http.StatusBadRequest)
		return
	}
	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	services, err := zc.ListServices(r.Context(), projectID)
	if err != nil {
		http.Error(w, "list services: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.render(w, r, ServiceSelect(services))
}

func (s *Server) backupTagsPartial(w http.ResponseWriter, r *http.Request) {
	tokenID, err := strconv.ParseInt(r.URL.Query().Get("zerops_token_id"), 10, 64)
	if err != nil {
		http.Error(w, "choose a token", http.StatusBadRequest)
		return
	}
	serviceID := r.URL.Query().Get("service_stack_id")
	if serviceID == "" {
		http.Error(w, "choose a service", http.StatusBadRequest)
		return
	}
	zc, err := s.zeropsClientFor(r.Context(), tokenID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	backups, err := zc.ListBackups(r.Context(), serviceID)
	if err != nil {
		http.Error(w, "list backups: "+err.Error(), http.StatusBadGateway)
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
	s.render(w, r, TagPicker(tags))
}
