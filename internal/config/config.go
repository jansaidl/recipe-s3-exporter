// Package config loads runtime configuration from environment variables.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the exporter.
type Config struct {
	// DatabaseURL is the PostgreSQL connection string (required).
	DatabaseURL string
	// MasterKey is the 32-byte AES key used to encrypt secrets at rest.
	MasterKey []byte
	// Port the HTTP server listens on.
	Port string
	// ZeropsAPI is the base URL of the Zerops public REST API.
	ZeropsAPI string
	// ZeropsAuthScheme is the Authorization scheme used for Zerops API calls
	// ("Bearer" by default; "none"/"raw"/"" sends the token with no prefix).
	ZeropsAuthScheme string
	// Workers is the number of concurrent export workers.
	Workers int
	// SecureCookies marks the session cookie as Secure (HTTPS only).
	SecureCookies bool

	// AdminEmail / AdminPassword bootstrap the first admin user when the
	// users table is empty. Optional.
	AdminEmail    string
	AdminPassword string
}

// Load reads configuration from the environment, applying defaults and
// validating required values.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:      firstNonEmpty(os.Getenv("DATABASE_URL"), os.Getenv("DB_URL")),
		Port:             getEnv("PORT", "8080"),
		ZeropsAPI:        strings.TrimRight(getEnv("ZEROPS_API", "https://api.app-prg1.zerops.io/api/rest/public"), "/"),
		ZeropsAuthScheme: getEnv("ZEROPS_AUTH_SCHEME", "Bearer"),
		AdminEmail:       strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:    os.Getenv("ADMIN_PASSWORD"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	key, err := parseMasterKey(os.Getenv("MASTER_KEY"))
	if err != nil {
		return nil, err
	}
	c.MasterKey = key

	workers := getEnv("EXPORT_WORKERS", "2")
	c.Workers, err = strconv.Atoi(workers)
	if err != nil || c.Workers < 1 {
		return nil, fmt.Errorf("EXPORT_WORKERS must be a positive integer, got %q", workers)
	}

	c.SecureCookies = parseBool(os.Getenv("SECURE_COOKIES"))

	return c, nil
}

// parseMasterKey accepts a 32-byte key encoded as standard or URL-safe base64,
// or as 64 hex characters. It is required so the app never runs without
// encryption configured.
func parseMasterKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("MASTER_KEY is required (base64 of 32 random bytes; generate with: openssl rand -base64 32)")
	}

	// Accept a raw 32-character key directly (e.g. Zerops generateRandomString(32)).
	if len(raw) == 32 {
		return []byte(raw), nil
	}

	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	} {
		if b, err := dec(raw); err == nil && len(b) == 32 {
			return b, nil
		}
	}

	return nil, fmt.Errorf("MASTER_KEY must decode to exactly 32 bytes (base64); got an invalid or wrong-length value")
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
