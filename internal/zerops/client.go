// Package zerops is a thin client for the Zerops public REST API covering the
// endpoints needed to discover services and export their backups.
package zerops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAPI is the Zerops public REST API base URL.
const DefaultAPI = "https://api.app-prg1.zerops.io/api/rest/public"

// DefaultAuthScheme is the Authorization scheme used per the Zerops OpenAPI
// spec (Bearer with an opaque token).
const DefaultAuthScheme = "Bearer"

// Client talks to the Zerops API with a single integration/access token.
type Client struct {
	base   string
	token  string
	scheme string
	http   *http.Client
}

// New builds a client. If base is empty, DefaultAPI is used. An optional auth
// scheme overrides the default "Bearer"; pass "" (or "none"/"raw") to send the
// raw token in the Authorization header with no scheme prefix.
func New(base, token string, scheme ...string) *Client {
	if base == "" {
		base = DefaultAPI
	}
	s := DefaultAuthScheme
	if len(scheme) > 0 {
		s = strings.TrimSpace(scheme[0])
	}
	return &Client{
		base:   strings.TrimRight(base, "/"),
		token:  token,
		scheme: s,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// authHeader builds the Authorization header value for the configured scheme.
func (c *Client) authHeader() string {
	switch strings.ToLower(c.scheme) {
	case "", "none", "raw":
		return c.token
	default:
		return c.scheme + " " + c.token
	}
}

// Project is a Zerops project.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Service is a Zerops service stack.
type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	TypeName string `json:"-"`
}

// Backup is a single backup file entry for a service.
type Backup struct {
	Name     string   // backup id (a date string)
	Path     string   // path inside the S3 bucket
	Size     int64    // bytes
	Filename string   // human filename from metadata
	Stack    string   // stack name from metadata
	Mode     string   // AUTOMATIC / MANUAL
	Tags     []string // tags attached to the backup
}

// GetClientID resolves the account (client) id associated with the token.
func (c *Client) GetClientID(ctx context.Context) (string, error) {
	var resp struct {
		ClientUserList []struct {
			ClientID string `json:"clientId"`
		} `json:"clientUserList"`
	}
	if err := c.do(ctx, http.MethodGet, "/user/info", nil, &resp); err != nil {
		return "", err
	}
	if len(resp.ClientUserList) == 0 || resp.ClientUserList[0].ClientID == "" {
		return "", fmt.Errorf("token is not associated with any client/account")
	}
	return resp.ClientUserList[0].ClientID, nil
}

// Validate confirms the token works and returns the client id.
func (c *Client) Validate(ctx context.Context) (string, error) {
	return c.GetClientID(ctx)
}

// ListProjects lists projects for a client.
func (c *Client) ListProjects(ctx context.Context, clientID string) ([]Project, error) {
	var resp struct {
		List []Project `json:"list"`
	}
	if err := c.do(ctx, http.MethodGet, "/client/"+url.PathEscape(clientID)+"/project", nil, &resp); err != nil {
		return nil, err
	}
	return resp.List, nil
}

// ListServices lists service stacks within a project.
func (c *Client) ListServices(ctx context.Context, projectID string) ([]Service, error) {
	var resp struct {
		List []struct {
			ID                   string `json:"id"`
			Name                 string `json:"name"`
			Status               string `json:"status"`
			ServiceStackTypeInfo struct {
				ServiceStackTypeName string `json:"serviceStackTypeName"`
			} `json:"serviceStackTypeInfo"`
		} `json:"list"`
	}
	if err := c.do(ctx, http.MethodGet, "/project/"+url.PathEscape(projectID)+"/service-stack", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(resp.List))
	for _, s := range resp.List {
		out = append(out, Service{ID: s.ID, Name: s.Name, Status: s.Status, TypeName: s.ServiceStackTypeInfo.ServiceStackTypeName})
	}
	return out, nil
}

// ListBackups lists the backups available for a service stack.
func (c *Client) ListBackups(ctx context.Context, serviceStackID string) ([]Backup, error) {
	var resp struct {
		Files []struct {
			Name     string          `json:"name"`
			Path     string          `json:"path"`
			Size     int64           `json:"size"`
			Metadata json.RawMessage `json:"metadata"`
		} `json:"files"`
	}
	if err := c.do(ctx, http.MethodGet, "/service-stack/"+url.PathEscape(serviceStackID)+"/backup", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Backup, 0, len(resp.Files))
	for _, f := range resp.Files {
		b := Backup{Name: f.Name, Path: f.Path, Size: f.Size}
		b.Filename, b.Stack, b.Mode, b.Tags = parseMetadata(f.Metadata)
		out = append(out, b)
	}
	return out, nil
}

// CreateDownloadURL requests a temporary download URL for a backup. backupName
// is the backup's "name" field (a date string).
func (c *Client) CreateDownloadURL(ctx context.Context, serviceStackID, backupName string) (string, error) {
	path := "/service-stack/" + url.PathEscape(serviceStackID) + "/backup/download-url/" + url.PathEscape(backupName)
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.do(ctx, http.MethodPost, path, strings.NewReader("{}"), &resp); err != nil {
		return "", err
	}
	if resp.URL == "" {
		return "", fmt.Errorf("empty download url returned")
	}
	return resp.URL, nil
}

// Download opens the backup byte stream from a temporary download URL. The
// caller must close the returned ReadCloser. The returned int64 is the
// response Content-Length (-1 if unknown). The download URL is a presigned
// object-storage link, so no Authorization header is sent.
func (c *Client) Download(ctx context.Context, downloadURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, 0, err
	}
	log.Printf("[zerops] -> GET (download) %s", sanitizeURL(downloadURL))
	start := time.Now()
	// Long-lived stream; use a client without the short API timeout.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		log.Printf("[zerops] !! download transport error after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return nil, 0, err
	}
	finalURL := downloadURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	log.Printf("[zerops] <- download %s CL=%d type=%q encoding=%q final=%s",
		resp.Status, resp.ContentLength, resp.Header.Get("Content-Type"),
		resp.Header.Get("Content-Encoding"), sanitizeURL(finalURL))
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		log.Printf("[zerops] !! download error body: %s", truncate(string(body), 500))
		return nil, 0, fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp.Body, resp.ContentLength, nil
}

// sanitizeURL strips the query string (which may carry presigned credentials)
// for safe logging, keeping scheme+host+path.
func sanitizeURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}

// parseMetadata extracts known fields from the freeform backup metadata map.
// The map has string keys; values are usually strings but "tags" is an array.
func parseMetadata(raw json.RawMessage) (filename, stack, mode string, tags []string) {
	if len(raw) == 0 {
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	asString := func(key string) string {
		v, ok := m[key]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
		return ""
	}
	filename = asString("filename")
	stack = asString("stackname")
	mode = asString("mode")
	if v, ok := m["tags"]; ok {
		_ = json.Unmarshal(v, &tags) // tags is an array of strings
	}
	return
}

// do executes a JSON request against the Zerops API and decodes into out.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	log.Printf("[zerops] -> %s %s (auth scheme=%q, token=%s)", method, path, c.scheme, maskToken(c.token))
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[zerops] !! %s %s transport error after %s: %v", method, path, time.Since(start).Round(time.Millisecond), err)
		return fmt.Errorf("zerops request %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	log.Printf("[zerops] <- %s %s %s (%s, %d bytes)", method, path, resp.Status, time.Since(start).Round(time.Millisecond), len(data))
	if resp.StatusCode/100 != 2 {
		log.Printf("[zerops] !! %s %s error body: %s", method, path, truncate(string(data), 500))
		return fmt.Errorf("zerops api %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		log.Printf("[zerops] !! %s %s decode error: %v; body: %s", method, path, err, truncate(string(data), 500))
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}

// maskToken returns a token preview safe for logs (first/last few chars).
func maskToken(t string) string {
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "…" + t[len(t)-4:]
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…(truncated)"
	}
	return s
}
