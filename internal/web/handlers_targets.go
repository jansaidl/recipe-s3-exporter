package web

import (
	"net/http"
	"strings"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/models"
)

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.db.ListTargets(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, r, TargetsPage(TargetsVM{
		User:    auth.UserFrom(r.Context()),
		Flash:   s.flash(r),
		Targets: targets,
	}))
}

func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	endpoint := strings.TrimSpace(r.FormValue("endpoint"))
	bucket := strings.TrimSpace(r.FormValue("bucket"))
	accessKey := strings.TrimSpace(r.FormValue("access_key"))
	secretKey := r.FormValue("secret_key")
	region := strings.TrimSpace(r.FormValue("region"))
	if region == "" {
		region = "us-east-1"
	}
	prefix := strings.TrimSpace(r.FormValue("prefix"))
	if prefix == "" {
		prefix = "zerops-backups"
	}

	if name == "" || endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		s.setFlash(r, "Name, endpoint, bucket, access key and secret key are required.")
		http.Redirect(w, r, "/targets", http.StatusSeeOther)
		return
	}

	akEnc, err1 := s.cipher.Encrypt(accessKey)
	skEnc, err2 := s.cipher.Encrypt(secretKey)
	if err1 != nil || err2 != nil {
		s.setFlash(r, "Failed to encrypt credentials.")
		http.Redirect(w, r, "/targets", http.StatusSeeOther)
		return
	}

	t := &models.ExportTarget{
		Name:                name,
		Endpoint:            endpoint,
		Region:              region,
		Bucket:              bucket,
		Prefix:              prefix,
		AccessKeyCiphertext: akEnc,
		SecretKeyCiphertext: skEnc,
		UsePathStyle:        r.FormValue("use_path_style") == "1",
		UseSSL:              r.FormValue("use_ssl") == "1",
	}
	id, err := s.db.CreateTarget(r.Context(), t)
	if err != nil {
		s.setFlash(r, "Failed to save target: "+err.Error())
		http.Redirect(w, r, "/targets", http.StatusSeeOther)
		return
	}

	// Best-effort connectivity test for immediate feedback.
	if store, err := s.storeForTarget(r.Context(), id); err == nil {
		if err := store.Test(r.Context()); err != nil {
			s.setFlash(r, "Target saved, but connection test failed: "+err.Error())
			http.Redirect(w, r, "/targets", http.StatusSeeOther)
			return
		}
	}
	s.setFlash(r, "Target added and connection verified.")
	http.Redirect(w, r, "/targets", http.StatusSeeOther)
}

func (s *Server) testTarget(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	store, err := s.storeForTarget(r.Context(), id)
	if err != nil {
		s.setFlash(r, "Target not found or unreadable: "+err.Error())
		http.Redirect(w, r, "/targets", http.StatusSeeOther)
		return
	}
	if err := store.Test(r.Context()); err != nil {
		s.setFlash(r, "Connection test failed: "+err.Error())
	} else {
		s.setFlash(r, "Connection OK — bucket is reachable.")
	}
	http.Redirect(w, r, "/targets", http.StatusSeeOther)
}

func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteTarget(r.Context(), id); err != nil {
		s.setFlash(r, "Delete failed: "+err.Error())
	} else {
		s.setFlash(r, "Target deleted.")
	}
	http.Redirect(w, r, "/targets", http.StatusSeeOther)
}
