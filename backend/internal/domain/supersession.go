package domain

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

// Supersession records that CBP revoked or modified an earlier ruling.
// The map is built from the corpus by `go run ./cmd/corpus supersession`
// (D320) and embedded here, so every precedent filter sees the same map
// without new configuration.
type Supersession struct {
	// Ruling is the ruling that was revoked or modified.
	Ruling string `json:"ruling"`
	// By is the ruling that revoked or modified it.
	By string `json:"superseded_by"`
	// Date is By's issue date (YYYY-MM-DD). CBP's change takes effect
	// later (60 days after publication); the issue date is used because
	// CBP's new position is public from then on.
	Date string `json:"date"`
	// Kind is "revoked" or "modified".
	Kind string `json:"kind"`
	// Source names where the edge was read: "effect_on_other_rulings",
	// "hereby" (a "hereby revoked/modified" sentence elsewhere in the text),
	// "subject" (the ruling's subject line) or "reference" (a later ruling
	// stating that one ruling was modified by another).
	Source string `json:"source"`
	// SourceRuling is the ruling whose text the edge was read from.
	SourceRuling string `json:"source_ruling"`
	// Quote is the sentence the edge was read from (at most 300 characters).
	Quote string `json:"quote"`
	// Pools lists the precedent pools that hold Ruling ("general",
	// "usmca", "final2"); empty when no pool does.
	Pools []string `json:"pools,omitempty"`
}

// Time parses Date.
func (s Supersession) Time() (time.Time, error) { return time.Parse(time.DateOnly, s.Date) }

// SupersessionMap is the committed file shape.
type SupersessionMap struct {
	Version string         `json:"version"`
	Rule    string         `json:"rule"`
	Counts  map[string]int `json:"counts"`
	Edges   []Supersession `json:"edges"`
}

//go:embed supersession.json
var supersessionJSON []byte

// supersededAt maps a ruling to its earliest supersession date.
var supersededAt, supersededBy = mustSupersession(supersessionJSON)

func mustSupersession(b []byte) (map[string]time.Time, map[string]Supersession) {
	at, by, err := ParseSupersession(b)
	if err != nil {
		panic(err)
	}
	return at, by
}

// ParseSupersession reads a supersession map file and indexes the earliest
// edge for each ruling.
func ParseSupersession(b []byte) (map[string]time.Time, map[string]Supersession, error) {
	var m SupersessionMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, nil, fmt.Errorf("supersession: %w", err)
	}
	at := map[string]time.Time{}
	by := map[string]Supersession{}
	for _, e := range m.Edges {
		d, err := e.Time()
		if err != nil {
			return nil, nil, fmt.Errorf("supersession: %s: bad date %q", e.Ruling, e.Date)
		}
		if e.Kind != "revoked" && e.Kind != "modified" {
			return nil, nil, fmt.Errorf("supersession: %s: kind %q", e.Ruling, e.Kind)
		}
		if old, ok := at[e.Ruling]; !ok || d.Before(old) {
			at[e.Ruling] = d
			by[e.Ruling] = e
		}
	}
	return at, by, nil
}

// SupersededAsOf returns the earliest revocation or modification of ruling
// id dated strictly before at (D370). A zero at means "today": any known
// supersession counts. A ruling revoked on or after at was still CBP's
// position at that date and is not reported. The same-day case matters: when
// the case being backtested is itself the revoking ruling, its own change
// must not hide the ruling it revokes, just as a ruling dated on the case
// date is never usable as its precedent.
func SupersededAsOf(id string, at time.Time) (Supersession, bool) {
	d, ok := supersededAt[id]
	if !ok {
		return Supersession{}, false
	}
	if !at.IsZero() && !d.Before(at) {
		return Supersession{}, false
	}
	return supersededBy[id], true
}

// Note is the short text shown beside a superseded ruling, e.g.
// "revoked by H325607 on 2025-08-29".
func (s Supersession) Note() string {
	return s.Kind + " by " + s.By + " on " + s.Date
}
