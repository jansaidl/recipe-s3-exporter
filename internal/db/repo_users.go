package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"recipe-s3-exporter/internal/models"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

// CountUsers returns the number of dashboard users.
func (d *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a new user and returns its id.
func (d *DB) CreateUser(ctx context.Context, email, passwordHash string, role models.Role) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id`,
		email, passwordHash, string(role),
	).Scan(&id)
	return id, err
}

// GetUserByEmail looks up a user by email.
func (d *DB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return d.scanUser(d.Pool.QueryRow(ctx,
		`SELECT id, email, password_hash, role, created_at FROM users WHERE email = $1`, email))
}

// GetUser looks up a user by id.
func (d *DB) GetUser(ctx context.Context, id int64) (*models.User, error) {
	return d.scanUser(d.Pool.QueryRow(ctx,
		`SELECT id, email, password_hash, role, created_at FROM users WHERE id = $1`, id))
}

// ListUsers returns all users ordered by creation.
func (d *DB) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, email, password_hash, role, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// DeleteUser removes a user by id.
func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

// SetUserRole updates a user's role.
func (d *DB) SetUserRole(ctx context.Context, id int64, role models.Role) error {
	_, err := d.Pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, id, string(role))
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (d *DB) scanUser(row pgx.Row) (*models.User, error) {
	u, err := scanUserRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func scanUserRow(row rowScanner) (*models.User, error) {
	var u models.User
	var role string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &role, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.Role = models.Role(role)
	return &u, nil
}
