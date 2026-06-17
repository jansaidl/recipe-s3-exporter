package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"recipe-s3-exporter/internal/models"
)

// CreateToken stores an encrypted Zerops integration token.
func (d *DB) CreateToken(ctx context.Context, name, ciphertext, clientID string) (int64, error) {
	var id int64
	now := time.Now()
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO zerops_tokens (name, token_ciphertext, client_id, last_validated_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		name, ciphertext, clientID, now,
	).Scan(&id)
	return id, err
}

// ListTokens returns all stored tokens (ciphertext included for internal use).
func (d *DB) ListTokens(ctx context.Context) ([]models.ZeropsToken, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, name, token_ciphertext, client_id, last_validated_at, created_at
		 FROM zerops_tokens ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ZeropsToken
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetToken fetches a single token by id.
func (d *DB) GetToken(ctx context.Context, id int64) (*models.ZeropsToken, error) {
	t, err := scanToken(d.Pool.QueryRow(ctx,
		`SELECT id, name, token_ciphertext, client_id, last_validated_at, created_at
		 FROM zerops_tokens WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// TouchTokenValidated records a successful validation and refreshes client id.
func (d *DB) TouchTokenValidated(ctx context.Context, id int64, clientID string) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE zerops_tokens SET last_validated_at = now(), client_id = $2 WHERE id = $1`,
		id, clientID)
	return err
}

// DeleteToken removes a token by id.
func (d *DB) DeleteToken(ctx context.Context, id int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM zerops_tokens WHERE id = $1`, id)
	return err
}

func scanToken(row rowScanner) (*models.ZeropsToken, error) {
	var t models.ZeropsToken
	if err := row.Scan(&t.ID, &t.Name, &t.TokenCiphertext, &t.ClientID, &t.LastValidatedAt, &t.CreatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}
