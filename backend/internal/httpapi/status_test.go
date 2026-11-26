package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func getStatus(t *testing.T, s *StatusPage) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestStatusPageCachesChecksAndHidesErrors(t *testing.T) {
	var runs atomic.Int32
	var now atomic.Int64
	now.Store(time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC).Unix())
	s := &StatusPage{
		Checks: []HealthCheck{
			{Name: "case_store", Run: func(context.Context) error { runs.Add(1); return nil }},
			{Name: "model", Run: func(context.Context) error { return errors.New("provider secret detail") }},
		},
		Every: time.Minute,
		SLOs:  DefaultSLOs,
		Now:   func() time.Time { return time.Unix(now.Load(), 0) },
	}
	first := getStatus(t, s)
	if first["status"] != "unknown" {
		t.Fatalf("first answer %v, want unknown before any snapshot", first["status"])
	}
	waitFor(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot != nil })
	for i := 0; i < 20; i++ {
		getStatus(t, s)
	}
	if runs.Load() != 1 {
		t.Fatalf("checks ran %d times within the interval, want 1", runs.Load())
	}
	out := getStatus(t, s)
	if out["status"] != "degraded" {
		t.Fatalf("status %v, want degraded", out["status"])
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "secret") {
		t.Fatal("a check error reached the public status")
	}
	if len(out["slos"].([]any)) != len(DefaultSLOs) {
		t.Fatal("SLOs missing")
	}
	now.Add(120)
	getStatus(t, s)
	waitFor(t, func() bool { return runs.Load() == 2 })
}
