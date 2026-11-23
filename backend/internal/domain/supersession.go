package domain

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
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
	// Bulletin cites the Customs Bulletin notice that published the change
	// (D400). For source "customs_bulletin" Date is the notice's effective
	// date; an edge read from a corpus letter keeps the letter's date and
	// gains the citation when a fetched notice confirms it.
	Bulletin *BulletinCite `json:"bulletin,omitempty"`
}

// BulletinCite names a notice in CBP's Customs Bulletin and Decisions.
type BulletinCite struct {
	Volume int    `json:"volume"`
	Number int    `json:"number"`
	Date   string `json:"date"` // issue date
	Page   int    `json:"page"` // printed page the notice starts on (0: unknown)
	URL    string `json:"url"`
	// Effective is the date a final change takes effect; CommentsBy is a
	// proposal's comment deadline.
	Effective  string `json:"effective,omitempty"`
	CommentsBy string `json:"comments_by,omitempty"`
}

// String is a short citation, e.g. "Customs Bulletin vol. 58 no. 20, p. 1".
func (c BulletinCite) String() string {
	s := fmt.Sprintf("Customs Bulletin vol. %d no. %d", c.Volume, c.Number)
	if c.Page > 0 {
		s += fmt.Sprintf(", p. %d", c.Page)
	}
	return s
}

// ProposedChange is a revocation or modification CBP proposed in the
// Customs Bulletin and has not, in the fetched notices, made final (D400).
// It is shown beside the ruling and never bars it as a precedent.
type ProposedChange struct {
	Ruling   string       `json:"ruling"`
	By       string       `json:"proposed_by"`
	Kind     string       `json:"kind"` // "revoked" or "modified"
	Quote    string       `json:"quote"`
	Bulletin BulletinCite `json:"bulletin"`
	Pools    []string     `json:"pools,omitempty"`
}

// Note is the short text shown beside the ruling, e.g. "CBP proposed to
// revoke it (H341085, Customs Bulletin vol. 58 no. 44, p. 1)".
func (p ProposedChange) Note() string {
	verb := map[string]string{"revoked": "revoke", "modified": "modify"}[p.Kind]
	return "CBP proposed to " + verb + " it (" + p.By + ", " + p.Bulletin.String() + ")"
}

// Time parses Date.
func (s Supersession) Time() (time.Time, error) { return time.Parse(time.DateOnly, s.Date) }

// SupersessionMap is the committed file shape.
type SupersessionMap struct {
	Version string         `json:"version"`
	Rule    string         `json:"rule"`
	Counts  map[string]int `json:"counts"`
	Edges   []Supersession `json:"edges"`
	// Proposed lists changes CBP proposed and has not made final in the
	// fetched notices (D400): shown, not enforced.
	Proposed []ProposedChange `json:"proposed,omitempty"`
}

//go:embed supersession.json
var supersessionJSON []byte

// supersededAt maps a ruling to its earliest supersession date.
var supersededAt, supersededBy = mustSupersession(supersessionJSON)

var proposedFor = mustProposed(supersessionJSON)

func mustSupersession(b []byte) (map[string]time.Time, map[string]Supersession) {
	at, by, err := ParseSupersession(b)
	if err != nil {
		panic(err)
	}
	return at, by
}

func mustProposed(b []byte) map[string]ProposedChange {
	var m SupersessionMap
	if err := json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	out := map[string]ProposedChange{}
	for _, p := range m.Proposed {
		if p.Kind != "revoked" && p.Kind != "modified" {
			panic(fmt.Sprintf("supersession: proposed %s: kind %q", p.Ruling, p.Kind))
		}
		if old, ok := out[p.Ruling]; !ok || p.Bulletin.Date < old.Bulletin.Date {
			out[p.Ruling] = p
		}
	}
	return out
}

// ProposedFor returns CBP's pending proposal to revoke or modify ruling id,
// if a fetched Customs Bulletin notice published one (D400). It is
// information only: a proposal never bars a precedent.
func ProposedFor(id string) (ProposedChange, bool) {
	p, ok := proposedFor[id]
	return p, ok
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
// "revoked by H325607 on 2025-08-29", or for a change read from the Customs
// Bulletin "revoked by H325364 effective 2024-07-22 (Customs Bulletin vol.
// 58 no. 20, p. 1)".
func (s Supersession) Note() string {
	if s.Source == SourceCustomsBulletin && s.Bulletin != nil {
		return s.Kind + " by " + s.By + " effective " + s.Date + " (" + s.Bulletin.String() + ")"
	}
	return s.Kind + " by " + s.By + " on " + s.Date
}

// SourceCustomsBulletin marks an edge read from a Customs Bulletin notice.
const SourceCustomsBulletin = "customs_bulletin"

// SupersessionFingerprint is a deterministic digest of the enforced
// supersession index: each ruling with its earliest supersession date, which
// is all SupersededAsOf decides on. Proposals, quotes, sources, the version
// string and edge order do not change it; adding, removing or re-dating an
// enforced edge does.
func SupersessionFingerprint(at map[string]time.Time) string {
	ids := make([]string, 0, len(at))
	for id := range at {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(id + " " + at[id].Format(time.DateOnly) + "\n")
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

var fingerprintOnce = sync.OnceValue(func() string { return SupersessionFingerprint(supersededAt) })

// EmbeddedSupersessionFingerprint is SupersessionFingerprint of the embedded
// map. It is computed once, after the map is parsed, and never empty.
func EmbeddedSupersessionFingerprint() string { return fingerprintOnce() }
