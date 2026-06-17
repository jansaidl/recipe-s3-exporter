package web

import (
	"net/http"
	"strings"

	"recipe-s3-exporter/internal/auth"
	"recipe-s3-exporter/internal/models"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, r, UsersPage(UsersVM{
		User:  auth.UserFrom(r.Context()),
		Flash: s.flash(r),
		Users: users,
	}))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	role := models.Role(r.FormValue("role"))
	if role != models.RoleAdmin {
		role = models.RoleViewer
	}
	if email == "" || len(password) < 8 {
		s.setFlash(r, "Email and a password of at least 8 characters are required.")
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		s.setFlash(r, "Failed to hash password.")
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	if _, err := s.db.CreateUser(r.Context(), email, hash, role); err != nil {
		s.setFlash(r, "Failed to create user (email may already exist): "+err.Error())
	} else {
		s.setFlash(r, "User created.")
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

func (s *Server) setUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	role := models.Role(r.URL.Query().Get("role"))
	if role != models.RoleAdmin && role != models.RoleViewer {
		s.setFlash(r, "Invalid role.")
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	// Prevent removing the last admin.
	if role == models.RoleViewer {
		if onlyAdmin, _ := s.isLastAdmin(r, id); onlyAdmin {
			s.setFlash(r, "Cannot demote the last admin.")
			http.Redirect(w, r, "/users", http.StatusSeeOther)
			return
		}
	}
	if err := s.db.SetUserRole(r.Context(), id, role); err != nil {
		s.setFlash(r, "Failed to update role: "+err.Error())
	} else {
		s.setFlash(r, "Role updated.")
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if me := auth.UserFrom(r.Context()); me != nil && me.ID == id {
		s.setFlash(r, "You cannot delete your own account.")
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	if onlyAdmin, _ := s.isLastAdmin(r, id); onlyAdmin {
		s.setFlash(r, "Cannot delete the last admin.")
		http.Redirect(w, r, "/users", http.StatusSeeOther)
		return
	}
	if err := s.db.DeleteUser(r.Context(), id); err != nil {
		s.setFlash(r, "Delete failed: "+err.Error())
	} else {
		s.setFlash(r, "User deleted.")
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

// isLastAdmin reports whether id is an admin and the only one left.
func (s *Server) isLastAdmin(r *http.Request, id int64) (bool, error) {
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		return false, err
	}
	admins := 0
	targetIsAdmin := false
	for _, u := range users {
		if u.IsAdmin() {
			admins++
			if u.ID == id {
				targetIsAdmin = true
			}
		}
	}
	return targetIsAdmin && admins <= 1, nil
}
