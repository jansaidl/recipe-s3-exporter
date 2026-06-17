package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"recipe-s3-exporter/internal/models"
)

// CreateRun inserts a new run row in the pending state and returns its id.
func (d *DB) CreateRun(ctx context.Context, jobID *int64) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO export_runs (job_id, status) VALUES ($1, $2) RETURNING id`,
		jobID, string(models.StatusPending),
	).Scan(&id)
	return id, err
}

// StartRun marks a run as running with the resolved backup metadata.
func (d *DB) StartRun(ctx context.Context, id int64, backupName string, size int64, s3Key string) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_runs SET status=$2, backup_name=$3, backup_size=$4, s3_key=$5, started_at=now()
		 WHERE id=$1`,
		id, string(models.StatusRunning), backupName, size, s3Key)
	return err
}

// UpdateRunProgress persists transferred bytes for a running export.
func (d *DB) UpdateRunProgress(ctx context.Context, id, bytes int64) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_runs SET bytes_transferred=$2 WHERE id=$1`, id, bytes)
	return err
}

// FinishRun marks a run terminal (success/failed/skipped) with an optional error.
func (d *DB) FinishRun(ctx context.Context, id int64, status models.RunStatus, bytes int64, errMsg string) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_runs SET status=$2, bytes_transferred=$3, error=$4, finished_at=now()
		 WHERE id=$1`,
		id, string(status), bytes, errMsg)
	return err
}

// GetRun fetches a run by id.
func (d *DB) GetRun(ctx context.Context, id int64) (*models.ExportRun, error) {
	r, err := scanRun(d.Pool.QueryRow(ctx, runSelect+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// ListRecentRuns returns the most recent runs up to limit.
func (d *DB) ListRecentRuns(ctx context.Context, limit int) ([]models.ExportRun, error) {
	return d.queryRuns(ctx, runSelect+` ORDER BY r.started_at DESC LIMIT $1`, limit)
}

// ListActiveRuns returns runs that are pending or running.
func (d *DB) ListActiveRuns(ctx context.Context) ([]models.ExportRun, error) {
	return d.queryRuns(ctx,
		runSelect+` WHERE r.status IN ('pending','running') ORDER BY r.started_at DESC`)
}

// MarkStaleRunsFailed flags any pending/running rows as failed. Used on startup
// to clean up runs orphaned by a crash or restart.
func (d *DB) MarkStaleRunsFailed(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_runs SET status='failed', error='interrupted by restart', finished_at=now()
		 WHERE status IN ('pending','running')`)
	return err
}

func (d *DB) queryRuns(ctx context.Context, sql string, args ...any) ([]models.ExportRun, error) {
	rows, err := d.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExportRun
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

const runSelect = `SELECT r.id, r.job_id, COALESCE(j.name,''), r.status, r.backup_name,
	r.backup_size, r.bytes_transferred, r.s3_key, r.error, r.started_at, r.finished_at
	FROM export_runs r LEFT JOIN export_jobs j ON j.id = r.job_id`

func scanRun(row rowScanner) (*models.ExportRun, error) {
	var r models.ExportRun
	var status string
	if err := row.Scan(&r.ID, &r.JobID, &r.JobName, &status, &r.BackupName,
		&r.BackupSize, &r.BytesTransferred, &r.S3Key, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
		return nil, err
	}
	r.Status = models.RunStatus(status)
	return &r, nil
}
