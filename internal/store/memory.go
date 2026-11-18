package store

import (
	"context"
	"sync"
	"time"
)

// Memory keeps links in a map. Data is lost on restart.
// The mutex matters: Go serves requests concurrently, and concurrent
// writes to a map crash the program.
type Memory struct {
	mu    sync.Mutex
	links map[string]*Link
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{links: make(map[string]*Link)}
}

func (m *Memory) Create(_ context.Context, code, url string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.links[code]; exists {
		return Link{}, ErrCodeTaken
	}
	link := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	m.links[code] = link
	return *link, nil
}

func (m *Memory) Resolve(_ context.Context, code string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, ok := m.links[code]
	if !ok {
		return "", ErrNotFound
	}
	link.Clicks++
	return link.URL, nil
}

func (m *Memory) Get(_ context.Context, code string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, ok := m.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return *link, nil // a copy, so callers can't change the stored link
}

func (m *Memory) Ping(context.Context) error { return nil }
