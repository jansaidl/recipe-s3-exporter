package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"recipe-s3-exporter/internal/models"
)

// CreateJob inserts a new export job.
func (d *DB) CreateJob(ctx context.Context, j *models.ExportJob) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO export_jobs
		   (name, zerops_token_id, project_id, project_name, service_stack_id, service_name,
		    target_id, tag_filter, schedule_cron, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		j.Name, j.ZeropsTokenID, j.ProjectID, j.ProjectName, j.ServiceStackID, j.ServiceName,
		j.TargetID, j.TagFilter, j.ScheduleCron, j.Enabled,
	).Scan(&id)
	return id, err
}

// UpdateJob updates an existing job's configuration.
func (d *DB) UpdateJob(ctx context.Context, j *models.ExportJob) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_jobs SET
		   name=$2, zerops_token_id=$3, project_id=$4, project_name=$5, service_stack_id=$6,
		   service_name=$7, target_id=$8, tag_filter=$9, schedule_cron=$10, enabled=$11
		 WHERE id=$1`,
		j.ID, j.Name, j.ZeropsTokenID, j.ProjectID, j.ProjectName, j.ServiceStackID,
		j.ServiceName, j.TargetID, j.TagFilter, j.ScheduleCron, j.Enabled,
	)
	return err
}

// SetJobEnabled toggles a job on or off.
func (d *DB) SetJobEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := d.Pool.Exec(ctx, `UPDATE export_jobs SET enabled=$2 WHERE id=$1`, id, enabled)
	return err
}

// MarkJobRun records the last and next scheduled run timestamps.
func (d *DB) MarkJobRun(ctx context.Context, id int64, lastRun time.Time, nextRun *time.Time) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE export_jobs SET last_run_at=$2, next_run_at=$3 WHERE id=$1`, id, lastRun, nextRun)
	return err
}

// SetJobNextRun records only the next scheduled run time (for display).
func (d *DB) SetJobNextRun(ctx context.Context, id int64, nextRun *time.Time) error {
	_, err := d.Pool.Exec(ctx, `UPDATE export_jobs SET next_run_at=$2 WHERE id=$1`, id, nextRun)
	return err
}

// DeleteJob removes a job by id.
func (d *DB) DeleteJob(ctx context.Context, id int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM export_jobs WHERE id = $1`, id)
	return err
}

// ListJobs returns all jobs joined with their target and token names.
func (d *DB) ListJobs(ctx context.Context) ([]models.ExportJob, error) {
	rows, err := d.Pool.Query(ctx, jobSelect+` ORDER BY j.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExportJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// ListEnabledJobs returns only enabled jobs (used by the scheduler).
func (d *DB) ListEnabledJobs(ctx context.Context) ([]models.ExportJob, error) {
	rows, err := d.Pool.Query(ctx, jobSelect+` WHERE j.enabled ORDER BY j.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExportJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// GetJob fetches a single job by id.
func (d *DB) GetJob(ctx context.Context, id int64) (*models.ExportJob, error) {
	j, err := scanJob(d.Pool.QueryRow(ctx, jobSelect+` WHERE j.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return j, err
}

const jobSelect = `SELECT j.id, j.name, j.zerops_token_id, j.project_id, j.project_name,
	j.service_stack_id, j.service_name, j.target_id, j.tag_filter, j.schedule_cron,
	j.enabled, j.last_run_at, j.next_run_at, j.created_at,
	COALESCE(t.name,''), COALESCE(zt.name,'')
	FROM export_jobs j
	LEFT JOIN export_targets t ON t.id = j.target_id
	LEFT JOIN zerops_tokens zt ON zt.id = j.zerops_token_id`

func scanJob(row rowScanner) (*models.ExportJob, error) {
	var j models.ExportJob
	if err := row.Scan(&j.ID, &j.Name, &j.ZeropsTokenID, &j.ProjectID, &j.ProjectName,
		&j.ServiceStackID, &j.ServiceName, &j.TargetID, &j.TagFilter, &j.ScheduleCron,
		&j.Enabled, &j.LastRunAt, &j.NextRunAt, &j.CreatedAt,
		&j.TargetName, &j.TokenName); err != nil {
		return nil, err
	}
	return &j, nil
}
