package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weeeijin/snip/internal/store"
)

func newTestServer() http.Handler {
	return New(store.NewMemory(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateRedirectAndCountClicks(t *testing.T) {
	h := newTestServer()

	rec := do(t, h, http.MethodPost, "/api/links", `{"url":"https://example.com/page"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want 201 (body %q)", rec.Code, rec.Body)
	}
	var link store.Link
	if err := json.Unmarshal(rec.Body.Bytes(), &link); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if len(link.Code) != codeLength {
		t.Fatalf("code %q has length %d, want %d", link.Code, len(link.Code), codeLength)
	}

	for range 2 {
		rec = do(t, h, http.MethodGet, "/"+link.Code, "")
		if rec.Code != http.StatusFound {
			t.Fatalf("redirect: got %d, want 302", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "https://example.com/page" {
			t.Fatalf("Location = %q", got)
		}
	}

	rec = do(t, h, http.MethodGet, "/api/links/"+link.Code, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: got %d, want 200", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &link); err != nil {
		t.Fatalf("decode stats response: %v", err)
	}
	if link.Clicks != 2 {
		t.Fatalf("clicks = %d, want 2", link.Clicks)
	}
}

func TestRejectsBadInput(t *testing.T) {
	h := newTestServer()
	cases := map[string]string{
		"not json":           `not json`,
		"missing url":        `{}`,
		"relative url":       `{"url":"/just/a/path"}`,
		"javascript scheme":  `{"url":"javascript:alert(1)"}`,
		"ftp scheme":         `{"url":"ftp://example.com/file"}`,
		"url over the limit": `{"url":"https://example.com/` + strings.Repeat("a", maxURLLength) + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, h, http.MethodPost, "/api/links", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
		})
	}
}

func TestUnknownCodeIs404(t *testing.T) {
	h := newTestServer()
	for _, path := range []string{"/zzzzzz", "/api/links/zzzzzz"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: got %d, want 404", path, rec.Code)
		}
	}
}

func TestHealthAndReadiness(t *testing.T) {
	h := newTestServer()
	for _, path := range []string{"/healthz", "/readyz"} {
		if rec := do(t, h, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", path, rec.Code)
		}
	}
}

func TestWrongMethodIs405(t *testing.T) {
	h := newTestServer()
	if rec := do(t, h, http.MethodPost, "/healthz", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", rec.Code)
	}
}

func TestNewCodeUsesOnlyAlphabet(t *testing.T) {
	for range 1000 {
		for _, c := range newCode() {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("unexpected character %q", c)
			}
		}
	}
}
