// Package models holds the domain types persisted in PostgreSQL.
package models

import "time"

// Role enumerates dashboard user roles.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

// User is a dashboard account.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// ZeropsToken is a stored Zerops integration token (encrypted at rest).
type ZeropsToken struct {
	ID              int64
	Name            string
	TokenCiphertext string
	ClientID        string
	LastValidatedAt *time.Time
	CreatedAt       time.Time
}

// ExportTarget is an S3-compatible destination bucket (secret key encrypted).
type ExportTarget struct {
	ID                  int64
	Name                string
	Endpoint            string // host[:port], e.g. s3.eu-central-1.amazonaws.com or a MinIO host
	Region              string
	Bucket              string
	Prefix              string
	AccessKeyCiphertext string
	SecretKeyCiphertext string
	UsePathStyle        bool
	UseSSL              bool
	CreatedAt           time.Time
}

// ExportJob is a configured, schedulable export rule.
type ExportJob struct {
	ID             int64
	Name           string
	ZeropsTokenID  int64
	ProjectID      string
	ProjectName    string
	ServiceStackID string
	ServiceName    string
	TargetID       int64
	TagFilter      []string // export the latest backup carrying ALL of these tags; empty = any
	ScheduleCron   string
	Enabled        bool
	LastRunAt      *time.Time
	NextRunAt      *time.Time
	CreatedAt      time.Time

	// Joined fields (read-only, populated by list queries).
	TargetName string
	TokenName  string
}

// RunStatus enumerates export run lifecycle states.
type RunStatus string

const (
	StatusPending RunStatus = "pending"
	StatusRunning RunStatus = "running"
	StatusSuccess RunStatus = "success"
	StatusFailed  RunStatus = "failed"
	StatusSkipped RunStatus = "skipped"
)

// ExportRun records a single execution and drives live progress.
type ExportRun struct {
	ID               int64
	JobID            *int64
	JobName          string
	Status           RunStatus
	BackupName       string
	BackupSize       int64
	BytesTransferred int64
	S3Key            string
	Error            string
	StartedAt        time.Time
	FinishedAt       *time.Time
}

// Progress returns transfer completion as a 0-100 percentage.
func (r ExportRun) Progress() int {
	if r.BackupSize <= 0 {
		return 0
	}
	p := int(r.BytesTransferred * 100 / r.BackupSize)
	if p > 100 {
		return 100
	}
	return p
}
