// Package cross is a client for CBP's CROSS rulings API at rulings.cbp.gov.
// The JSON API is public but undocumented; endpoints here were checked live
// on 15 Oct 2026: GET /api/ruling/<id> returns one ruling with its full text.
package cross

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultBaseURL is the public CROSS site.
const DefaultBaseURL = "https://rulings.cbp.gov"

// Ruling is a CROSS ruling as returned by /api/ruling/<id>. Only the fields
// Unde uses are decoded.
type Ruling struct {
	RulingNumber string `json:"rulingNumber"`
	Subject      string `json:"subject"`
	Categories   string `json:"categories"`
	RulingDate   string `json:"rulingDate"` // e.g. "2026-01-29T00:00:00"
	Collection   string `json:"collection"`
	Text         string `json:"text"`
}

// Date parses RulingDate.
func (r Ruling) Date() (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05", r.RulingDate)
}

// Client reads rulings from CROSS.
type Client interface {
	Ruling(ctx context.Context, id string) (Ruling, error)
	// Search returns up to pageSize rulings matching term, newest first. When
	// toDate is set, only rulings dated on or before it are returned (CROSS's
	// toDate is inclusive; checked live 15 Oct 2026).
	Search(ctx context.Context, term string, toDate time.Time, pageSize int) ([]SearchHit, error)
}

// SearchHit is one CROSS search result.
type SearchHit struct {
	RulingNumber string `json:"rulingNumber"`
	RulingDate   string `json:"rulingDate"`
	Subject      string `json:"subject"`
	Collection   string `json:"collection"`
	Categories   string `json:"categories,omitempty"`
	Tariffs      string `json:"tariffs,omitempty"` // HTS numbers, comma-separated
}

// Date parses RulingDate.
func (h SearchHit) Date() (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05", h.RulingDate)
}

// ErrNotFound means CROSS has no ruling with that number.
var ErrNotFound = errors.New("cross: ruling not found")

// UserAgent identifies this app to rulings.cbp.gov and hts.usitc.gov.
const UserAgent = "unde-labs (Google Cloud AI Builder Cup 2026 prototype)"

// HTTPClient is the live CROSS client.
type HTTPClient struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
}

// NewHTTPClient returns a client for the public site.
func NewHTTPClient(userAgent string) *HTTPClient {
	return &HTTPClient{BaseURL: DefaultBaseURL, UserAgent: userAgent, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

// Ruling fetches one ruling by number.
func (c *HTTPClient) Ruling(ctx context.Context, id string) (Ruling, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/ruling/"+id, nil)
	if err != nil {
		return Ruling{}, fmt.Errorf("cross: ruling %s: %w", id, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Ruling{}, fmt.Errorf("cross: ruling %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Ruling{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return Ruling{}, fmt.Errorf("cross: ruling %s: HTTP %d: %s", id, resp.StatusCode, b)
	}
	var r Ruling
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return Ruling{}, fmt.Errorf("cross: ruling %s: decode: %w", id, err)
	}
	if r.RulingNumber == "" {
		// CROSS answers 404 for unknown numbers (checked 15 Oct 2026); treat an
		// empty body the same way defensively.
		return Ruling{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return r, nil
}

// Search implements Client via GET /api/search.
func (c *HTTPClient) Search(ctx context.Context, term string, toDate time.Time, pageSize int) ([]SearchHit, error) {
	q := url.Values{
		"term": {term}, "collection": {"ALL"}, "commodityGrouping": {"ALL"},
		"pageSize": {strconv.Itoa(pageSize)}, "page": {"1"}, "sortBy": {"RELEVANCE"},
	}
	if !toDate.IsZero() {
		q.Set("toDate", toDate.Format("2006-01-02"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/search?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("cross: search %q: %w", term, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cross: search %q: %w", term, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("cross: search %q: HTTP %d: %s", term, resp.StatusCode, b)
	}
	var out struct {
		Rulings []SearchHit `json:"rulings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("cross: search %q: decode: %w", term, err)
	}
	return out.Rulings, nil
}

// SearchPage returns one page (from 1) of rulings matching term, newest
// first, and the total number of hits. Used to collect a regime's rulings
// (corpus usmca, D61); the product's precedent search uses Search.
func (c *HTTPClient) SearchPage(ctx context.Context, term string, page, pageSize int) ([]SearchHit, int, error) {
	q := url.Values{
		"term": {term}, "collection": {"ALL"}, "commodityGrouping": {"ALL"},
		"pageSize": {strconv.Itoa(pageSize)}, "page": {strconv.Itoa(page)}, "sortBy": {"DATE_DESC"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/search?"+q.Encode(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("cross: search %q page %d: %w", term, page, err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("cross: search %q page %d: %w", term, page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, 0, fmt.Errorf("cross: search %q page %d: HTTP %d: %s", term, page, resp.StatusCode, b)
	}
	var out struct {
		Rulings   []SearchHit `json:"rulings"`
		TotalHits int         `json:"totalHits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, 0, fmt.Errorf("cross: search %q page %d: decode: %w", term, page, err)
	}
	return out.Rulings, out.TotalHits, nil
}

var _ Client = (*HTTPClient)(nil)
