package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

// Searcher is the verifier's view of the ruling database. Every call is
// bounded: only rulings dated strictly before `before` and not in exclude.
type Searcher interface {
	Search(ctx context.Context, query string, before time.Time, exclude map[string]bool) ([]Hit, error)
	// Read returns a ruling's facts and CBP's analysis/holding as text.
	Read(ctx context.Context, id string, before time.Time, exclude map[string]bool) (string, error)
}

// Hit is one search result. Origin is "?" when unknown (CROSS search does not
// return the held origin).
type Hit struct {
	ID      string
	Date    time.Time
	Subject string
	Origin  string
}

// SearchHits is how many hits a search returns (agent_search.py: 8).
const SearchHits = 8

// notFound is the tool answer for a ruling outside the allowed window.
const notFound = "NOT FOUND (only rulings dated before the case are available)."

// Local searches the local corpus, mirroring the Corpus class in
// hackathon-research/experiments/origin-proof/agent_search.py: BM25 over
// subject + first 3,000 characters of facts for every labelled ruling with a
// facts file; Read returns the facts (2,500 characters) and the ruling text
// after the facts cut (2,500 characters).
type Local struct {
	items  []domain.Ruling
	byID   map[string]int
	bm     *retrieval.BM25
	rawDir string
}

// NewLocal builds a Local searcher. rawDir holds CROSS JSON (for the
// analysis text read_ruling returns).
func NewLocal(labels []corpus.Label, factsDir, rawDir string) (*Local, error) {
	l := &Local{byID: map[string]int{}, rawDir: rawDir}
	for _, lab := range labels {
		b, err := os.ReadFile(filepath.Join(factsDir, lab.Ruling+".txt"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("verifier: read facts %s: %w", lab.Ruling, err)
		}
		date, _ := lab.Time()
		origin := lab.Origin()
		if origin == "" {
			origin = "?"
		}
		l.byID[lab.Ruling] = len(l.items)
		l.items = append(l.items, domain.Ruling{ID: lab.Ruling, Date: date, Subject: lab.Subject, Facts: string(b), Origin: origin})
	}
	l.bm = retrieval.NewBM25(l.items, func(r domain.Ruling) string {
		return r.Subject + " " + corpus.Truncate(r.Facts, retrieval.DocRunes)
	})
	return l, nil
}

// Search implements Searcher.
func (l *Local) Search(_ context.Context, query string, before time.Time, exclude map[string]bool) ([]Hit, error) {
	var out []Hit
	for _, h := range l.bm.Top(query, SearchHits, func(r domain.Ruling) bool { return r.Date.Before(before) && !exclude[r.ID] }) {
		out = append(out, Hit{ID: h.Ruling.ID, Date: h.Ruling.Date, Subject: h.Ruling.Subject, Origin: h.Ruling.Origin})
	}
	return out, nil
}

var spaces = regexp.MustCompile(`\s+`)

// Read implements Searcher.
func (l *Local) Read(_ context.Context, id string, before time.Time, exclude map[string]bool) (string, error) {
	i, ok := l.byID[id]
	if !ok || !l.items[i].Date.Before(before) || exclude[id] {
		return notFound, nil
	}
	r := l.items[i]
	analysis := ""
	if b, err := os.ReadFile(filepath.Join(l.rawDir, id+".json")); err == nil {
		var raw cross.Ruling
		if json.Unmarshal(b, &raw) == nil {
			// As in agent_search.py: the facts cut (an offset in the
			// normalised text) is applied to the raw text.
			_, meta := corpus.SplitFacts(raw.Text)
			rest := []rune(raw.Text)
			if meta.End < len(rest) {
				analysis = corpus.Truncate(spaces.ReplaceAllString(string(rest[meta.End:]), " "), 2500)
			}
		}
	}
	return fmt.Sprintf("[%s | %s] %s\nFACTS: %s\nCBP ANALYSIS AND HOLDING: %s",
		r.ID, r.Date.Format(time.DateOnly), r.Subject, corpus.Truncate(r.Facts, 2500), analysis), nil
}

// CROSS searches the live CROSS database (production). toDate is inclusive,
// so it asks for rulings up to the day before the case and filters again.
type CROSS struct {
	Client cross.Client
}

// Search implements Searcher.
func (c CROSS) Search(ctx context.Context, query string, before time.Time, exclude map[string]bool) ([]Hit, error) {
	hits, err := c.Client.Search(ctx, query, before.AddDate(0, 0, -1), SearchHits*2)
	if err != nil {
		return nil, err
	}
	var out []Hit
	for _, h := range hits {
		d, err := h.Date()
		if err != nil || !d.Before(before) || exclude[h.RulingNumber] {
			continue
		}
		out = append(out, Hit{ID: h.RulingNumber, Date: d, Subject: h.Subject, Origin: "?"})
		if len(out) == SearchHits {
			break
		}
	}
	return out, nil
}

// Read implements Searcher.
func (c CROSS) Read(ctx context.Context, id string, before time.Time, exclude map[string]bool) (string, error) {
	if exclude[id] || !domain.ValidRulingID(id) {
		return notFound, nil
	}
	r, err := c.Client.Ruling(ctx, id)
	if errors.Is(err, cross.ErrNotFound) {
		return notFound, nil
	}
	if err != nil {
		return "", err
	}
	d, err := r.Date()
	if err != nil || !d.Before(before) {
		return notFound, nil
	}
	facts, meta := corpus.SplitFacts(r.Text)
	rest := []rune(spaces.ReplaceAllString(r.Text, " "))
	analysis := ""
	if meta.End < len(rest) {
		analysis = corpus.Truncate(string(rest[meta.End:]), 2500)
	}
	return fmt.Sprintf("[%s | %s] %s\nFACTS: %s\nCBP ANALYSIS AND HOLDING: %s",
		r.RulingNumber, d.Format(time.DateOnly), strings.TrimSpace(r.Subject), corpus.Truncate(facts, 2500), analysis), nil
}

// formatHits renders search results as agent_search.py does.
func formatHits(hits []Hit) string {
	if len(hits) == 0 {
		return "no results"
	}
	lines := make([]string, len(hits))
	for i, h := range hits {
		lines[i] = fmt.Sprintf("%s | %s | %s | CBP held: %s", h.ID, h.Date.Format(time.DateOnly), corpus.Truncate(h.Subject, 110), h.Origin)
	}
	return strings.Join(lines, "\n")
}

var rulingRef = regexp.MustCompile(`\b[NH][0-9]{6}\b`)
