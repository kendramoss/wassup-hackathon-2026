package cross

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRuling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/api/ruling/N357771":
			_, _ = io.WriteString(w, `{"rulingNumber":"N357771","subject":"The country of origin of a heat gun","rulingDate":"2026-01-29T00:00:00","text":"Dear Mr. X: ..."}`)
		case "/api/search":
			q := r.URL.Query()
			if q.Get("term") != "heat gun" || q.Get("toDate") != "2026-01-13" || q.Get("pageSize") != "8" {
				t.Errorf("search query = %v", q)
			}
			_, _ = io.WriteString(w, `{"rulings":[{"rulingNumber":"N345897","rulingDate":"2025-03-03T00:00:00","subject":"The country of origin of a heat gun"}],"totalHits":1}`)
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
	hits, err := c.Search(context.Background(), "heat gun", time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC), 8)
	if err != nil || len(hits) != 1 || hits[0].RulingNumber != "N345897" {
		t.Fatalf("Search = %+v, %v", hits, err)
	}
	if d, _ := hits[0].Date(); d.Format("2006-01-02") != "2025-03-03" {
		t.Errorf("hit date = %v", d)
	}
	for _, id := range []string{"N000000", "N999999"} {
		if _, err := c.Ruling(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestSearchPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/search" || q.Get("term") != "102.20" || q.Get("sortBy") != "DATE_DESC" || q.Get("page") != "3" ||
			q.Get("pageSize") != "100" || q.Get("collection") != "ALL" {
			t.Errorf("search request = %s %v", r.URL.Path, q)
		}
		_, _ = io.WriteString(w, `{"rulings":[{"rulingNumber":"N364405","rulingDate":"2026-09-17T00:00:00","subject":"bolt seal","tariffs":"8309.90.0080","categories":"Origin"}],"totalHits":1062}`)
	}))
	defer srv.Close()
	c := &HTTPClient{BaseURL: srv.URL, UserAgent: "test-agent", HTTP: srv.Client()}
	hits, total, err := c.SearchPage(context.Background(), "102.20", 3, 100)
	if err != nil || total != 1062 || len(hits) != 1 || hits[0].Tariffs != "8309.90.0080" || hits[0].Categories != "Origin" {
		t.Fatalf("SearchPage = %+v %d %v", hits, total, err)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer bad.Close()
	c.BaseURL, c.HTTP = bad.URL, bad.Client()
	if _, _, err := c.SearchPage(context.Background(), "102.20", 1, 100); err == nil {
		t.Error("403 accepted")
	}
}
