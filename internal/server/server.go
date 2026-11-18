// Package server is the HTTP layer: routes, input validation and responses.
package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/weeeijin/snip/internal/store"
)

const (
	codeLength   = 6
	maxURLLength = 2048
	maxBodyBytes = 4096
	// No 0/o/1/l: easy to confuse when typed. 32 symbols divide 256 evenly,
	// so every symbol is equally likely.
	alphabet = "abcdefghijkmnpqrstuvwxyz23456789"
)

// Server holds the dependencies the handlers need.
type Server struct {
	store store.Store
	log   *slog.Logger
}

// New returns the app's HTTP handler with every route registered.
func New(s store.Store, log *slog.Logger) http.Handler {
	srv := &Server{store: s, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", srv.healthz)
	mux.HandleFunc("GET /readyz", srv.readyz)
	mux.HandleFunc("POST /api/links", srv.createLink)
	mux.HandleFunc("GET /api/links/{code}", srv.getLink)
	mux.HandleFunc("GET /{code}", srv.redirect)
	return mux
}

// healthz: is the process alive? (Kubernetes restarts the pod if not.)
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok\n"))
}

// readyz: can it serve traffic right now? (Kubernetes stops sending
// traffic if not, without restarting.)
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		s.log.Warn("readiness check failed", "err", err)
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ready\n"))
}

// POST /api/links {"url": "https://..."} → 201 Link
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// A random 6-character code can collide with an existing one; retry.
	for range 3 {
		link, err := s.store.Create(r.Context(), newCode(), req.URL)
		if errors.Is(err, store.ErrCodeTaken) {
			continue
		}
		if err != nil {
			s.internalError(w, "create link", err)
			return
		}
		writeJSON(w, http.StatusCreated, link)
		return
	}
	s.internalError(w, "create link", errors.New("no unique code after 3 attempts"))
}

// GET /api/links/{code} → 200 Link (with click count)
func (s *Server) getLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.store.Get(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		s.internalError(w, "get link", err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// GET /{code} → 302 redirect to the long URL
func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	target, err := s.store.Resolve(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, "resolve link", err)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// validateURL only accepts absolute http(s) URLs. Without this, someone could
// shorten javascript: or data: URLs and use your domain for phishing or XSS.
func validateURL(raw string) error {
	if len(raw) > maxURLLength {
		return fmt.Errorf("url longer than %d characters", maxURLLength)
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be an absolute http:// or https:// URL")
	}
	return nil
}

// newCode returns a random short code using a cryptographically secure source,
// so codes can't be guessed.
func newCode() string {
	b := make([]byte, codeLength)
	_, _ = rand.Read(b) // never fails on supported platforms
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// internalError logs the real cause and returns a generic message, so
// internal details (SQL errors, hostnames) never leak to users.
func (s *Server) internalError(w http.ResponseWriter, op string, err error) {
	s.log.Error(op+" failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
