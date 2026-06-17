package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"recipe-s3-exporter/internal/models"
)

// CreateTarget stores an S3 export target (secrets already encrypted).
func (d *DB) CreateTarget(ctx context.Context, t *models.ExportTarget) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO export_targets
		   (name, endpoint, region, bucket, prefix, access_key_ciphertext, secret_key_ciphertext, use_path_style, use_ssl)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		t.Name, t.Endpoint, t.Region, t.Bucket, t.Prefix,
		t.AccessKeyCiphertext, t.SecretKeyCiphertext, t.UsePathStyle, t.UseSSL,
	).Scan(&id)
	return id, err
}

// ListTargets returns all export targets.
func (d *DB) ListTargets(ctx context.Context) ([]models.ExportTarget, error) {
	rows, err := d.Pool.Query(ctx, targetSelect+` ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExportTarget
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTarget fetches a single target by id.
func (d *DB) GetTarget(ctx context.Context, id int64) (*models.ExportTarget, error) {
	t, err := scanTarget(d.Pool.QueryRow(ctx, targetSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// DeleteTarget removes a target by id.
func (d *DB) DeleteTarget(ctx context.Context, id int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM export_targets WHERE id = $1`, id)
	return err
}

const targetSelect = `SELECT id, name, endpoint, region, bucket, prefix,
	access_key_ciphertext, secret_key_ciphertext, use_path_style, use_ssl, created_at
	FROM export_targets`

func scanTarget(row rowScanner) (*models.ExportTarget, error) {
	var t models.ExportTarget
	if err := row.Scan(&t.ID, &t.Name, &t.Endpoint, &t.Region, &t.Bucket, &t.Prefix,
		&t.AccessKeyCiphertext, &t.SecretKeyCiphertext, &t.UsePathStyle, &t.UseSSL, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
