package web

import (
	"net/http"
	"strings"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/zerops"
)

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.db.ListTokens(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, r, TokensPage(TokensVM{
		User:   auth.UserFrom(r.Context()),
		Flash:  s.flash(r),
		Tokens: tokens,
	}))
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	token := strings.TrimSpace(r.FormValue("token"))
	if name == "" || token == "" {
		s.setFlash(r, "Name and token are required.")
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}

	// Validate against the Zerops API before storing.
	clientID, err := zerops.New(s.cfg.ZeropsAPI, token).Validate(r.Context())
	if err != nil {
		s.setFlash(r, "Token validation failed: "+err.Error())
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}

	ciphertext, err := s.cipher.Encrypt(token)
	if err != nil {
		s.setFlash(r, "Failed to encrypt token: "+err.Error())
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	if _, err := s.db.CreateToken(r.Context(), name, ciphertext, clientID); err != nil {
		s.setFlash(r, "Failed to save token: "+err.Error())
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	s.setFlash(r, "Token added and validated (client "+clientID+").")
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (s *Server) validateToken(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	zc, err := s.zeropsClientFor(r.Context(), id)
	if err != nil {
		s.setFlash(r, "Token not found or unreadable: "+err.Error())
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	clientID, err := zc.Validate(r.Context())
	if err != nil {
		s.setFlash(r, "Validation failed: "+err.Error())
		http.Redirect(w, r, "/tokens", http.StatusSeeOther)
		return
	}
	_ = s.db.TouchTokenValidated(r.Context(), id, clientID)
	s.setFlash(r, "Token is valid (client "+clientID+").")
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteToken(r.Context(), id); err != nil {
		s.setFlash(r, "Delete failed: "+err.Error())
	} else {
		s.setFlash(r, "Token deleted.")
	}
	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}
