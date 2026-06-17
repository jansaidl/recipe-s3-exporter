// Package zerops is a thin client for the Zerops public REST API covering the
// endpoints needed to discover services and export their backups.
package zerops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultAPI is the Zerops public REST API base URL.
const DefaultAPI = "https://api.app-prg1.zerops.io/api/rest/public"

// Client talks to the Zerops API with a single integration/access token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New builds a client. If base is empty, DefaultAPI is used.
func New(base, token string) *Client {
	if base == "" {
		base = DefaultAPI
	}
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 60 * time.Second},
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
// caller must close the returned ReadCloser.
func (c *Client) Download(ctx context.Context, downloadURL string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, 0, err
	}
	// Long-lived stream; use a client without the short API timeout.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, 0, fmt.Errorf("download failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return resp.Body, resp.ContentLength, nil
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
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("zerops api %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}
