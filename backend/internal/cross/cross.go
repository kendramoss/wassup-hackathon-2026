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
	"time"
)

// DefaultBaseURL is the public CROSS site.
const DefaultBaseURL = "https://rulings.cbp.gov"

// Ruling is a CROSS ruling as returned by /api/ruling/<id>. Only the fields
// Origin Proof uses are decoded.
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

// Client reads rulings from CROSS. Search (with a toDate filter) is added for
// the verifier in Gate C.
type Client interface {
	Ruling(ctx context.Context, id string) (Ruling, error)
}

// ErrNotFound means CROSS has no ruling with that number.
var ErrNotFound = errors.New("cross: ruling not found")

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

var _ Client = (*HTTPClient)(nil)
