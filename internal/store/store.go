// Package store saves short links. The app talks to the Store interface, so
// the same code runs on an in-memory map (tests, quick local runs) or
// PostgreSQL (real deployments).
package store

import (
	"context"
	"errors"
	"time"
)

// Link is one shortened URL.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	// ErrNotFound means no link exists for the code.
	ErrNotFound = errors.New("link not found")
	// ErrCodeTaken means the generated code is already in use; pick another.
	ErrCodeTaken = errors.New("code already exists")
)

// Store is everything the HTTP layer needs from storage.
type Store interface {
	// Create saves a new link under code.
	Create(ctx context.Context, code, url string) (Link, error)
	// Resolve returns the long URL for code and counts one click.
	Resolve(ctx context.Context, code string) (string, error)
	// Get returns a link without counting a click.
	Get(ctx context.Context, code string) (Link, error)
	// Ping reports whether storage is reachable (used by /readyz).
	Ping(ctx context.Context) error
}
