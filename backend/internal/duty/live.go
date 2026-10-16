package duty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// DefaultHTSBaseURL is the USITC HTS site; its REST search answers JSON
// (checked 17 Oct 2026).
const DefaultHTSBaseURL = "https://hts.usitc.gov"

// AdditionalDutiesWarning goes with every live rate: the MFN rate is only
// the first component of what an import pays (D65).
const AdditionalDutiesWarning = "MFN (column 1 general) only: additional duties (Section 301, forced-labor, 232) not included — check before relying on the total"

// specialNote says what else the MFN rate leaves out.
const specialNote = "Special (free trade agreement, GSP) rates are not applied, and column 2 origins (Cuba, North Korea, Russia, Belarus) are not filled."

// ErrNoRate means the HTS has no line for the subheading.
var ErrNoRate = errors.New("duty: no HTS line")

// MFN is the column 1 general rate of one subheading, fetched live from the
// USITC HTS (D65). Rate is set only for a plain ad valorem rate ("4.2%",
// "Free"); a specific or compound rate ("4.4¢/kg", "20.7¢/kg + 7.5%") is
// reported as printed with Supported false, never converted.
type MFN struct {
	HTS         string    `json:"hts"`
	Description string    `json:"description"`
	General     string    `json:"general"` // as printed in the HTS
	Rate        *float64  `json:"rate"`    // ad valorem fraction; nil unless Supported
	Supported   bool      `json:"supported"`
	MFNOnly     bool      `json:"mfn_only"` // always true: no additional duties
	Live        bool      `json:"live"`     // always true: not from the researched table
	Source      string    `json:"source"`
	FetchedAt   time.Time `json:"fetched_at"`
	Note        string    `json:"note"`
}

// Live fetches MFN rates from the USITC HTS REST search and caches them in
// memory. Lookup failures are errors for the caller to treat as "no rate".
type Live struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
	// TTL is how long a fetched rate (or a missing line) is reused; FailTTL
	// how long a failed fetch is, so an outage does not cost every request
	// a timeout.
	TTL, FailTTL time.Duration

	now   func() time.Time
	mu    sync.Mutex
	cache map[string]liveEntry
}

type liveEntry struct {
	m     MFN
	err   error
	until time.Time
}

// maxLiveEntries bounds the cache (subheadings come from user input).
const maxLiveEntries = 2000

// NewLive returns a client for the public HTS with the given cache TTL and
// per-request timeout.
func NewLive(userAgent string, ttl, timeout time.Duration) *Live {
	return &Live{BaseURL: DefaultHTSBaseURL, UserAgent: userAgent, HTTP: &http.Client{Timeout: timeout}, TTL: ttl, FailTTL: time.Minute}
}

// LiveHTS returns the 8-digit subheading (NNNN.NN.NN) of an 8- or 10-digit
// HTS number, or false for anything else.
func LiveHTS(hts string) (string, bool) {
	var d strings.Builder
	for _, r := range hts {
		switch {
		case r >= '0' && r <= '9':
			d.WriteRune(r)
		case r == '.' || r == ' ':
		default:
			return "", false
		}
	}
	if n := d.Len(); n != 8 && n != 10 {
		return "", false
	}
	return Normalize(d.String()), true
}

// Cached answers from the cache only: ok is false when hts would need a
// fetch. An HTS number that is not 8 or 10 digits is answered (ErrNoRate).
func (l *Live) Cached(hts string) (m MFN, ok bool, err error) {
	sub, valid := LiveHTS(hts)
	if !valid {
		return MFN{}, true, fmt.Errorf("%w: %q is not an 8- or 10-digit HTS number", ErrNoRate, hts)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e, hit := l.cache[sub]
	if !hit || !l.clock().Before(e.until) {
		return MFN{}, false, nil
	}
	return e.m, true, e.err
}

// MFN returns the column 1 general rate for an 8- or 10-digit HTS number.
// ErrNoRate means the HTS has no such line; any other error is a failed
// fetch. Both are cached (failures for FailTTL).
func (l *Live) MFN(ctx context.Context, hts string) (MFN, error) {
	if m, ok, err := l.Cached(hts); ok {
		return m, err
	}
	sub, _ := LiveHTS(hts)
	now := l.clock()
	m, err := l.fetch(ctx, sub, now)
	if err != nil && ctx.Err() != nil {
		return MFN{}, err // the caller gave up; that says nothing about the HTS
	}
	ttl := l.TTL
	if err != nil && !errors.Is(err, ErrNoRate) {
		ttl = min(l.FailTTL, l.TTL)
	}
	l.mu.Lock()
	if l.cache == nil {
		l.cache = map[string]liveEntry{}
	}
	if len(l.cache) >= maxLiveEntries {
		for k, old := range l.cache {
			if !now.Before(old.until) {
				delete(l.cache, k)
			}
		}
	}
	if len(l.cache) < maxLiveEntries {
		l.cache[sub] = liveEntry{m: m, err: err, until: now.Add(ttl)}
	}
	l.mu.Unlock()
	return m, err
}

func (l *Live) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// htsRow is one row of GET /reststop/search.
type htsRow struct {
	HTSNo       string `json:"htsno"`
	Description string `json:"description"`
	General     string `json:"general"`
}

func (l *Live) fetch(ctx context.Context, sub string, now time.Time) (MFN, error) {
	src := l.BaseURL + "/reststop/search?keyword=" + url.QueryEscape(sub)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return MFN{}, fmt.Errorf("duty: hts %s: %w", sub, err)
	}
	req.Header.Set("User-Agent", l.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return MFN{}, fmt.Errorf("duty: hts %s: %w", sub, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return MFN{}, fmt.Errorf("duty: hts %s: HTTP %d: %s", sub, resp.StatusCode, b)
	}
	var rows []htsRow
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rows); err != nil {
		return MFN{}, fmt.Errorf("duty: hts %s: decode: %w", sub, err)
	}
	m, err := pick(rows, sub)
	if err != nil {
		return MFN{}, err
	}
	m.Source, m.FetchedAt = src, now.UTC()
	return m, nil
}

// pick finds the subheading's rate among the search rows. The search is
// fuzzy (0402.21.25 also returns 2106.90.03), so rows must match the 8
// digits. The rate is printed on the 8-digit row, or on its 10-digit row
// when the subheading has a single statistical line (8471.30.01.00);
// statistical rows below an 8-digit row print no rate.
func pick(rows []htsRow, sub string) (MFN, error) {
	want := strings.ReplaceAll(sub, ".", "")
	var exact *htsRow
	var stat []htsRow
	for i, r := range rows {
		d := strings.ReplaceAll(r.HTSNo, ".", "")
		switch {
		case d == want:
			exact = &rows[i]
		case len(d) == 10 && strings.HasPrefix(d, want) && strings.TrimSpace(r.General) != "":
			stat = append(stat, r)
		}
	}
	m := MFN{HTS: sub, MFNOnly: true, Live: true}
	switch {
	case exact != nil && strings.TrimSpace(exact.General) != "":
		m.Description, m.General = exact.Description, exact.General
	case len(stat) == 1:
		m.Description, m.General = stat[0].Description, stat[0].General
	case len(stat) > 1:
		// Statistical lines with their own rates: not one rate to give.
		m.Description = stat[0].Description
		m.Note = "statistical lines under " + sub + " print different rates; enter the rate yourself. " + AdditionalDutiesWarning + "."
		return m, nil
	case exact != nil:
		m.Description = exact.Description
		m.Note = "no column 1 general rate printed on " + sub + "; enter the rate yourself. " + AdditionalDutiesWarning + "."
		return m, nil
	default:
		return MFN{}, fmt.Errorf("%w: %s", ErrNoRate, sub)
	}
	m.General = strings.TrimSpace(m.General)
	if r, ok := ParseAdValorem(m.General); ok {
		m.Rate, m.Supported = &r, true
		m.Note = AdditionalDutiesWarning + ". " + specialNote
	} else {
		m.Note = fmt.Sprintf("general rate %q is specific or compound, not converted; enter the rate yourself. %s.", m.General, AdditionalDutiesWarning)
	}
	return m, nil
}

var adValorem = regexp.MustCompile(`^(\d{1,3}(?:\.\d+)?)%$`)

// ParseAdValorem parses a plain ad valorem HTS rate: "Free" is 0 and "4.2%"
// is 0.042. Anything else (specific, compound, "The duty provided in ...")
// is not parsed.
func ParseAdValorem(general string) (float64, bool) {
	g := strings.TrimSpace(general)
	if strings.EqualFold(g, "free") {
		return 0, true
	}
	sm := adValorem.FindStringSubmatch(g)
	if sm == nil {
		return 0, false
	}
	pct, err := strconv.ParseFloat(sm[1], 64)
	if err != nil || pct > 100 {
		return 0, false
	}
	return roundRate(pct / 100), true
}

func roundRate(r float64) float64 { return math.Round(r*1e6) / 1e6 }

// Column2 are the origins that pay the HTS column 2 rate, not MFN: Cuba and
// North Korea, and Russia and Belarus since the 2022 suspension of normal
// trade relations. A live MFN rate is never filled for them.
var Column2 = map[string]bool{"Cuba": true, "North Korea": true, "Russia": true, "Russian Federation": true, "Belarus": true}

// customsTerritory is where the goods are imported to: the customs territory
// of the United States (HTS General Note 2: the states, DC and Puerto Rico).
// Goods made there are not imported, so they owe no MFN duty on entry.
var customsTerritory = map[string]bool{"United States": true, "Puerto Rico": true}

// NoLiveRate reports whether a live MFN rate must never be filled for the
// normalised origin n: a column 2 origin, or a US one. A US plant or target
// is left unpriced (rather than given the import rate) so a plan never shows
// an MFN duty on goods that are not imported.
func NoLiveRate(n string) bool { return Column2[n] || customsTerritory[n] }

// Fill returns rates with the MFN rate added for each origin the user gave
// no rate for, except column 2 and US origins (NoLiveRate). It reports the
// origins it filled; nothing is filled unless the rate is Supported.
func (m MFN) Fill(rates map[string]float64, origins []string) (map[string]float64, []string) {
	if !m.Supported || m.Rate == nil {
		return rates, nil
	}
	out := map[string]float64{}
	given := map[string]bool{}
	for k, v := range rates {
		out[k] = v
		given[domain.NormalizeCountry(k)] = true
	}
	var filled []string
	for _, o := range origins {
		n := domain.NormalizeCountry(o)
		if n == "" || given[n] || NoLiveRate(n) {
			continue
		}
		given[n] = true
		out[n] = *m.Rate
		filled = append(filled, n)
	}
	return out, filled
}

// FillNote describes live-filled rates (or why none were) for
// Economics.DutyRatesNote.
func (m MFN) FillNote(filled []string) string {
	if !m.Supported {
		return fmt.Sprintf("Live HTS lookup for %s (%s, fetched %s): %s", m.HTS, m.Source, m.FetchedAt.Format(time.DateOnly), m.Note)
	}
	return fmt.Sprintf("Duty rates for %s are the live HTS MFN rate for %s, %s (%s, fetched %s). %s",
		strings.Join(filled, ", "), m.HTS, m.General, m.Source, m.FetchedAt.Format(time.DateOnly), m.Note)
}
