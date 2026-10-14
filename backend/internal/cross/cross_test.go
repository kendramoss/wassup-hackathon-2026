package cross

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRuling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/api/ruling/N357771":
			_, _ = io.WriteString(w, `{"rulingNumber":"N357771","subject":"The country of origin of a heat gun","rulingDate":"2026-01-29T00:00:00","text":"Dear Mr. X: ..."}`)
		case "/api/ruling/N000000":
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &HTTPClient{BaseURL: srv.URL, UserAgent: "test-agent", HTTP: srv.Client()}

	r, err := c.Ruling(context.Background(), "N357771")
	if err != nil || r.Subject != "The country of origin of a heat gun" {
		t.Fatalf("Ruling = %+v, %v", r, err)
	}
	if d, err := r.Date(); err != nil || d.Format("2006-01-02") != "2026-01-29" {
		t.Fatalf("Date = %v, %v", d, err)
	}
	for _, id := range []string{"N000000", "N999999"} {
		if _, err := c.Ruling(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", id, err)
		}
	}
}
