package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// RulingSearch finds earlier CBP rulings by text: the general engine's BM25
// index over the precedent corpus (retrieval.BM25 satisfies it).
type RulingSearch interface {
	Search(ctx context.Context, query string, k int, allow func(domain.Ruling) bool) ([]retrieval.Hit, error)
}

const (
	searchQueryMax   = 300 // characters
	searchLimitMax   = 20
	searchSnippetLen = 300 // characters of the facts section
)

type rulingHitDTO struct {
	ID      string  `json:"id"`
	Date    string  `json:"date"`
	Subject string  `json:"subject,omitempty"`
	Origin  *string `json:"origin"` // CBP's held origin; null when no single origin
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
	// Superseded is set when CBP revoked or modified the ruling on or
	// before `before` (any known supersession without it, D320). The ruling
	// is still listed so a reader can see it and the ruling that replaced it.
	Superseded *supersededDTO `json:"superseded,omitempty"`
}

// searchRulings answers GET /v1/rulings?q=&limit=&before= from the local
// corpus. No model call and no CBP fetch: it uses the per-client lookup
// bucket only. The query is never echoed or logged.
func (a *api) searchRulings(w http.ResponseWriter, r *http.Request) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "the query string could not be parsed", nil)
		return
	}
	text := strings.Join(strings.Fields(q.Get("q")), " ")
	if utf8.RuneCountInString(text) > searchQueryMax || len(retrieval.Tokens(text)) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_input",
			fmt.Sprintf("q must hold at least one word of 3 or more letters and at most %d characters", searchQueryMax), nil)
		return
	}
	limit := 10
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > searchLimitMax {
			writeError(w, http.StatusBadRequest, "invalid_input", fmt.Sprintf("limit must be 1 to %d", searchLimitMax), nil)
			return
		}
		limit = n
	}
	var before time.Time
	if v := q.Get("before"); v != "" {
		if before, err = time.Parse(time.DateOnly, v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_input", "before must be a date (YYYY-MM-DD)", nil)
			return
		}
	}
	allow := func(rl domain.Ruling) bool { return before.IsZero() || rl.Date.Before(before) }
	hits, err := a.d.RulingSearch.Search(r.Context(), text, limit, allow)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := make([]rulingHitDTO, 0, len(hits))
	for _, h := range hits {
		if h.Score <= 0 { // BM25 fills k with rulings that share no query word
			continue
		}
		out = append(out, rulingHitDTO{
			ID: h.Ruling.ID, Date: h.Ruling.Date.Format(time.DateOnly), Subject: h.Ruling.Subject,
			Origin: nonEmpty(h.Ruling.Origin), URL: domain.RulingURL(h.Ruling.ID),
			Snippet: snippet(h.Ruling.Facts, searchSnippetLen), Score: h.Score,
			Superseded: supersededAsOf(h.Ruling.ID, before),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rulings": out, "disclaimer": domain.Disclaimer})
}

// snippet is the first n characters of s with whitespace collapsed.
func snippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
