package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	var logs bytes.Buffer
	h := NewHandler(slog.New(slog.NewJSONHandler(&logs, nil)), Info{Provider: "fake"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || body["status"] != "ok" || body["llm_provider"] != "fake" {
		t.Fatalf("body = %v, err %v", body, err)
	}
	id := rec.Header().Get("X-Request-ID")
	if len(id) != 16 || !strings.Contains(logs.String(), id) {
		t.Fatalf("request id %q not set or not logged: %s", id, logs.String())
	}
}

func TestRequestIDFromCaller(t *testing.T) {
	h := NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), Info{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "abc123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc123" {
		t.Fatalf("X-Request-ID = %q", got)
	}
}

func TestNotFoundUsesErrorFormat(t *testing.T) {
	h := NewHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), Info{})
	for _, path := range []string{"/nope", "/healthz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		var body ErrorBody
		if rec.Code != http.StatusNotFound || json.NewDecoder(rec.Body).Decode(&body) != nil || body.Code != "not_found" {
			t.Errorf("%s: status %d body %+v", path, rec.Code, body)
		}
	}
}
