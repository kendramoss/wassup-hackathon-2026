package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlers(t *testing.T) {
	version = "abc1234"
	mux := newMux("2026-10-14T00:00:00Z")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<code>abc1234</code>") {
		t.Errorf("GET / = %d, body missing version", w.Code)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthy && (w.Code != http.StatusOK || w.Body.String() != "ok") {
		t.Errorf("GET /health = %d %q, want 200 ok", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", w.Code)
	}
}
