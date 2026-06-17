// Package auth handles password hashing, session management, the current-user
// context, and route protection middleware.
package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"recipe-s3-exporter/internal/db"
	"recipe-s3-exporter/internal/models"
)

type ctxKey int

const userKey ctxKey = iota

const sessionUserID = "user_id"

// Service ties together the session manager and the user store.
type Service struct {
	SM *scs.SessionManager
	DB *db.DB
}

// New creates the auth service. sqlDB is a database/sql handle (pgx stdlib)
// used by the session store; secure marks cookies HTTPS-only.
func New(sqlDB *sql.DB, store *db.DB, secure bool) *Service {
	sm := scs.New()
	sm.Store = postgresstore.New(sqlDB)
	sm.Lifetime = 12 * time.Hour
	sm.Cookie.Name = "s3exporter_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = secure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	return &Service{SM: sm, DB: store}
}

// HashPassword returns a bcrypt hash for the given plaintext password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword compares a plaintext password against a stored hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Login verifies credentials and, on success, establishes a session.
func (s *Service) Login(ctx context.Context, email, password string) (*models.User, error) {
	user, err := s.DB.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, err
	}
	if !CheckPassword(user.PasswordHash, password) {
		return nil, db.ErrNotFound // generic: avoid leaking which part failed
	}
	if err := s.SM.RenewToken(ctx); err != nil {
		return nil, err
	}
	s.SM.Put(ctx, sessionUserID, user.ID)
	return user, nil
}

// Logout destroys the current session.
func (s *Service) Logout(ctx context.Context) error {
	return s.SM.Destroy(ctx)
}

// LoadUser middleware resolves the logged-in user (if any) into the request
// context. It must run inside the session manager's LoadAndSave middleware.
func (s *Service) LoadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := s.SM.GetInt64(r.Context(), sessionUserID)
		if id > 0 {
			if user, err := s.DB.GetUser(r.Context(), id); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey, user))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth blocks unauthenticated requests, redirecting to /login.
func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserFrom(r.Context()) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin blocks non-admin users.
func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !u.IsAdmin() {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFrom returns the user stored in the context, or nil.
func UserFrom(ctx context.Context) *models.User {
	u, _ := ctx.Value(userKey).(*models.User)
	return u
}

// BootstrapAdmin creates the first admin from env credentials when no users
// exist yet. It is a no-op if users already exist or credentials are missing.
func (s *Service) BootstrapAdmin(ctx context.Context, email, password string) error {
	n, err := s.DB.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 || email == "" || password == "" {
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.DB.CreateUser(ctx, strings.ToLower(strings.TrimSpace(email)), hash, models.RoleAdmin)
	return err
}
