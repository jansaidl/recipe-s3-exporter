package web

import (
	"net/http"
	"strconv"
	"strings"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/models"
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.db.ListJobs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user := auth.UserFrom(r.Context())
	s.render(w, r, JobsPage(JobsVM{
		User:  user,
		Flash: s.flash(r),
		Jobs:  jobs,
		Admin: user.IsAdmin(),
	}))
}

func (s *Server) newJob(w http.ResponseWriter, r *http.Request) {
	s.renderJobForm(w, r, nil, "")
}

func (s *Server) renderJobForm(w http.ResponseWriter, r *http.Request, job *models.ExportJob, errMsg string) {
	tokens, _ := s.db.ListTokens(r.Context())
	targets, _ := s.db.ListTargets(r.Context())
	vm := JobFormVM{
		User:    auth.UserFrom(r.Context()),
		Flash:   s.flash(r),
		Tokens:  tokens,
		Targets: targets,
		Job:     job,
		Err:     errMsg,
	}
	if job == nil {
		s.render(w, r, NewJobPage(vm))
	} else {
		s.render(w, r, EditJobPage(vm))
	}
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	tokenID, _ := strconv.ParseInt(r.FormValue("zerops_token_id"), 10, 64)
	targetID, _ := strconv.ParseInt(r.FormValue("target_id"), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	projectID := r.FormValue("project_id")
	serviceID := r.FormValue("service_stack_id")

	if name == "" || tokenID == 0 || targetID == 0 || serviceID == "" {
		s.renderJobForm(w, r, nil, "Name, token, service and target are required.")
		return
	}

	job := &models.ExportJob{
		Name:           name,
		ZeropsTokenID:  tokenID,
		ProjectID:      projectID,
		ServiceStackID: serviceID,
		TargetID:       targetID,
		TagFilter:      cleanTags(r.Form["tags"]),
		ScheduleCron:   strings.TrimSpace(r.FormValue("schedule_cron")),
		Enabled:        r.FormValue("enabled") == "1",
	}

	// Resolve human-readable project/service names for display (best effort).
	if zc, err := s.zeropsClientFor(r.Context(), tokenID); err == nil {
		if projectID != "" {
			if tok, err := s.db.GetToken(r.Context(), tokenID); err == nil {
				cid := tok.ClientID
				if cid == "" {
					cid, _ = zc.GetClientID(r.Context())
				}
				if projects, err := zc.ListProjects(r.Context(), cid); err == nil {
					for _, p := range projects {
						if p.ID == projectID {
							job.ProjectName = p.Name
						}
					}
				}
			}
			if services, err := zc.ListServices(r.Context(), projectID); err == nil {
				for _, sv := range services {
					if sv.ID == serviceID {
						job.ServiceName = sv.Name
					}
				}
			}
		}
	}

	if _, err := s.db.CreateJob(r.Context(), job); err != nil {
		s.renderJobForm(w, r, nil, "Failed to save job: "+err.Error())
		return
	}
	_ = s.sched.Reload(r.Context())
	s.setFlash(r, "Job created.")
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (s *Server) editJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	job, err := s.db.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	s.renderJobForm(w, r, job, "")
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	job, err := s.db.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	job.Name = strings.TrimSpace(r.FormValue("name"))
	job.ScheduleCron = strings.TrimSpace(r.FormValue("schedule_cron"))
	job.Enabled = r.FormValue("enabled") == "1"
	job.TagFilter = cleanTags(strings.Split(r.FormValue("tags"), ","))
	if tid, err := strconv.ParseInt(r.FormValue("target_id"), 10, 64); err == nil && tid > 0 {
		job.TargetID = tid
	}
	if job.Name == "" {
		s.renderJobForm(w, r, job, "Name is required.")
		return
	}

	if err := s.db.UpdateJob(r.Context(), job); err != nil {
		s.renderJobForm(w, r, job, "Failed to save: "+err.Error())
		return
	}
	_ = s.sched.Reload(r.Context())
	s.setFlash(r, "Job updated.")
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (s *Server) toggleJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	job, err := s.db.GetJob(r.Context(), id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if err := s.db.SetJobEnabled(r.Context(), id, !job.Enabled); err != nil {
		s.setFlash(r, "Toggle failed: "+err.Error())
	} else {
		_ = s.sched.Reload(r.Context())
		s.setFlash(r, "Job "+enabledWord(!job.Enabled)+".")
	}
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteJob(r.Context(), id); err != nil {
		s.setFlash(r, "Delete failed: "+err.Error())
	} else {
		_ = s.sched.Reload(r.Context())
		s.setFlash(r, "Job deleted.")
	}
	http.Redirect(w, r, "/jobs", http.StatusSeeOther)
}

func (s *Server) runJob(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if _, err := s.pool.Enqueue(r.Context(), id); err != nil {
		s.setFlash(r, "Failed to enqueue run: "+err.Error())
	} else {
		s.setFlash(r, "Export queued. Watch progress on the dashboard.")
	}
	http.Redirect(w, r, "/runs", http.StatusSeeOther)
}

// cleanTags trims, lowercases-as-is, and drops empty tag entries.
func cleanTags(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func enabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
