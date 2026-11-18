package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres stores links in PostgreSQL through a connection pool.
type Postgres struct {
	pool *pgxpool.Pool
}

const schema = `
CREATE TABLE IF NOT EXISTS links (
	code       TEXT PRIMARY KEY,
	url        TEXT        NOT NULL,
	clicks     BIGINT      NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// uniqueViolation is PostgreSQL's error code for a duplicate primary key.
const uniqueViolation = "23505"

// NewPostgres connects using a URL like
// postgres://user:password@host:5432/dbname?sslmode=disable
// and creates the table if it doesn't exist yet.
func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Close releases all database connections.
func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) Create(ctx context.Context, code, url string) (Link, error) {
	var link Link
	err := p.pool.QueryRow(ctx,
		`INSERT INTO links (code, url) VALUES ($1, $2)
		 RETURNING code, url, clicks, created_at`,
		code, url,
	).Scan(&link.Code, &link.URL, &link.Clicks, &link.CreatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return Link{}, ErrCodeTaken
	}
	return link, err
}

// Resolve counts the click and returns the URL in a single query.
func (p *Postgres) Resolve(ctx context.Context, code string) (string, error) {
	var url string
	err := p.pool.QueryRow(ctx,
		`UPDATE links SET clicks = clicks + 1 WHERE code = $1 RETURNING url`,
		code,
	).Scan(&url)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return url, err
}

func (p *Postgres) Get(ctx context.Context, code string) (Link, error) {
	var link Link
	err := p.pool.QueryRow(ctx,
		`SELECT code, url, clicks, created_at FROM links WHERE code = $1`,
		code,
	).Scan(&link.Code, &link.URL, &link.Clicks, &link.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return link, err
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
