// Package supplierdecl holds the rules of supplier origin declarations
// (#916): what a supplier may answer, when a declaration holds, how it is
// compared with the value the user typed, and the sourced fact it adds to a
// case. It does no I/O. The routes are in internal/httpapi.
package supplierdecl

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Limits.
const (
	// MaxParts is how many parts one request may ask about.
	MaxParts = 50
	// MaxRequests caps the requests kept on one case, MaxActive the open ones.
	MaxRequests = 20
	MaxActive   = 5
	// MaxAnswers caps the answers kept on one request.
	MaxAnswers = 200
	// MaxName caps a supplier or signer name, MaxTitle a signer's title,
	// MaxHowMade the short text on how the part was made, in characters.
	MaxName    = 120
	MaxTitle   = 120
	MaxHowMade = 600
	// DefaultDays and MaxDays bound a link's life.
	DefaultDays = 14
	MaxDays     = 60
	// MaxValidYears bounds how far ahead a declaration may run.
	MaxValidYears = 5
	// MaxDocBytes caps the attached document. It is far below the media
	// check's 8 MB because the bytes travel in a JSON body and sit in the
	// verdict-cache collection, whose documents are 1 MiB at most.
	MaxDocBytes = 128 << 10
	// DefaultExpiringDays and MaxExpiringDays bound the expiry listing.
	DefaultExpiringDays = 30
	MaxExpiringDays     = 365
)

// Status of a declaration as the requester sees it.
const (
	StatusValid   = "valid"
	StatusExpired = "expired"
	StatusRevoked = "revoked"
	// StatusStale: the parts list changed after the declaration was given,
	// so the name at that position is no longer the part declared for.
	StatusStale = "stale"
)

// Source attributes the fact in the case's facts and in the API.
const Source = "supplier-declared"

var emailPattern = regexp.MustCompile(`^[^@\s]{1,64}@[^@\s]{1,190}\.[^@\s]{2,}$`)

// namePattern is what an origin outside the country table may look like: a
// few words of letters. The declared origin goes into a model's input, so
// nothing longer or odder than a place name is accepted from outside.
var namePattern = regexp.MustCompile(`^\p{L}[\p{L} .'()-]{1,39}$`)

// countryLike reports whether every country of a normalised origin ("A/B")
// is in the country table or at least reads as a short place name.
func countryLike(origin string) bool {
	for _, c := range domain.CountryParts(origin) {
		if _, ok := domain.KnownCountry(c); ok {
			continue
		}
		if !namePattern.MatchString(c) || len(strings.Fields(c)) > 4 {
			return false
		}
	}
	return true
}

var htsPattern = regexp.MustCompile(`^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`)

// CleanLine is single-line plain text: valid UTF-8, no control characters,
// runs of space folded, at most max characters. It returns an error naming
// the field, never the value.
func CleanLine(s, field string, max int, required bool) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: %s is not valid UTF-8", domain.ErrInvalid, field)
	}
	s = strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, s)), " ")
	if s == "" && required {
		return "", fmt.Errorf("%w: %s is required", domain.ErrInvalid, field)
	}
	if n := utf8.RuneCountInString(s); n > max {
		return "", fmt.Errorf("%w: %s is %d characters; the most is %d", domain.ErrInvalid, field, n, max)
	}
	return s, nil
}

// CleanEmail checks a supplier email. It is stored, never logged.
func CleanEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len(s) > 254 || !emailPattern.MatchString(s) {
		return "", fmt.Errorf("%w: supplier_email is not an email address", domain.ErrInvalid)
	}
	return s, nil
}

// Answer is what the supplier sends for one part.
type Answer struct {
	Origin      string
	HTS         string
	HowMade     string
	SignerName  string
	SignerTitle string
	ValidUntil  string
}

// Clean validates an answer and returns it ready to store. now bounds
// ValidUntil: not before today, not more than MaxValidYears ahead.
func Clean(in Answer, now time.Time) (store.SupplierAnswer, error) {
	var out store.SupplierAnswer
	origin := domain.NormalizeCountry(strings.TrimSpace(in.Origin))
	if origin == "" || utf8.RuneCountInString(in.Origin) > 80 {
		return out, fmt.Errorf("%w: origin (a country) is required", domain.ErrInvalid)
	}
	if !countryLike(origin) {
		return out, fmt.Errorf("%w: origin must be a country name", domain.ErrInvalid)
	}
	out.Origin = origin
	if h := strings.TrimSpace(in.HTS); h != "" {
		if !htsPattern.MatchString(h) {
			return out, fmt.Errorf("%w: hts is not an HTS heading or subheading", domain.ErrInvalid)
		}
		out.HTS = h
	}
	var err error
	if out.HowMade, err = CleanLine(in.HowMade, "how_made", MaxHowMade, true); err != nil {
		return out, err
	}
	if out.SignerName, err = CleanLine(in.SignerName, "signer_name", MaxName, true); err != nil {
		return out, err
	}
	if out.SignerTitle, err = CleanLine(in.SignerTitle, "signer_title", MaxTitle, true); err != nil {
		return out, err
	}
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(in.ValidUntil))
	if err != nil {
		return out, fmt.Errorf("%w: valid_until must be a date, YYYY-MM-DD", domain.ErrInvalid)
	}
	today := now.UTC().Truncate(24 * time.Hour)
	if d.Before(today) {
		return out, fmt.Errorf("%w: valid_until is in the past", domain.ErrInvalid)
	}
	if d.After(today.AddDate(MaxValidYears, 0, 0)) {
		return out, fmt.Errorf("%w: valid_until is more than %d years ahead", domain.ErrInvalid, MaxValidYears)
	}
	out.ValidUntil = d.Format(time.DateOnly)
	return out, nil
}

// Open reports whether a request's link still works at now.
func Open(rq store.SupplierRequest, now time.Time) bool {
	return rq.RevokedAt == nil && now.Before(rq.ExpiresAt)
}

// ActiveCount is the requests whose link works at now.
func ActiveCount(rqs []store.SupplierRequest, now time.Time) int {
	n := 0
	for _, rq := range rqs {
		if Open(rq, now) {
			n++
		}
	}
	return n
}

// Declaration is one part's declaration in effect, with its status.
type Declaration struct {
	Request  string // the request's id
	Supplier string
	store.SupplierAnswer
	Status string
}

// DeclaredOn is the day the answer was given.
func (d Declaration) DeclaredOn() string { return d.AnsweredAt.UTC().Format(time.DateOnly) }

// expired reports whether the last valid day is over at now.
func (d Declaration) expired(now time.Time) bool {
	v, err := time.Parse(time.DateOnly, d.ValidUntil)
	return err != nil || !now.UTC().Before(v.Add(24*time.Hour))
}

// DaysLeft is the whole days from today to the last valid day (0 on the
// day itself, negative once past).
func (d Declaration) DaysLeft(now time.Time) int {
	v, err := time.Parse(time.DateOnly, d.ValidUntil)
	if err != nil {
		return -1
	}
	return int(v.Sub(now.UTC().Truncate(24*time.Hour)).Hours() / 24)
}

// Current is the declaration in effect for each part (by position in the
// product's parts). A valid declaration beats any other (a lapsed, revoked
// or stale one is shown only when nothing valid exists for the part); among
// equals the newest wins. parts are the product's part names now.
func Current(rqs []store.SupplierRequest, parts []string, now time.Time) map[int]Declaration {
	out := map[int]Declaration{}
	for _, rq := range rqs {
		for _, a := range rq.Answers {
			if a.Part < 0 || a.Part >= len(parts) {
				continue
			}
			d := Declaration{Request: rq.ID, Supplier: rq.SupplierName, SupplierAnswer: a}
			switch {
			case !strings.EqualFold(parts[a.Part], a.PartName):
				d.Status = StatusStale
			case rq.RevokedAt != nil:
				d.Status = StatusRevoked
			case d.expired(now):
				d.Status = StatusExpired
			default:
				d.Status = StatusValid
			}
			if cur, ok := out[a.Part]; !ok || better(d, cur) {
				out[a.Part] = d
			}
		}
	}
	return out
}

// better: a valid declaration beats any other; among equals the newer wins.
func better(d, cur Declaration) bool {
	if (d.Status == StatusValid) != (cur.Status == StatusValid) {
		return d.Status == StatusValid
	}
	return !d.AnsweredAt.Before(cur.AnsweredAt)
}

// Fact is a declared origin set beside the value the user typed for the
// part. Both are always shown; neither replaces the other.
type Fact struct {
	Part     int
	PartName string
	// UserOrigin is the part's made_in as the case lists it.
	UserOrigin string
	Declaration
	// Differs is true when the declared origin is not the user's value.
	Differs bool
}

// Facts pairs the declaration in effect of every part with the user's
// value, in parts order. Stale declarations are left out.
func Facts(p *domain.Product, rqs []store.SupplierRequest, now time.Time) []Fact {
	if p == nil || len(rqs) == 0 {
		return nil
	}
	names := make([]string, len(p.Components))
	for i, c := range p.Components {
		names[i] = c.Name
	}
	cur := Current(rqs, names, now)
	idx := make([]int, 0, len(cur))
	for i := range cur {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	var out []Fact
	for _, i := range idx {
		d := cur[i]
		if d.Status == StatusStale {
			continue
		}
		user := domain.NormalizeCountry(p.Components[i].MadeIn)
		out = append(out, Fact{Part: i, PartName: p.Components[i].Name, UserOrigin: user, Declaration: d, Differs: !strings.EqualFold(user, d.Origin)})
	}
	return out
}

// FactsBlock is the text added to a case's facts for the predict input: one
// line per part whose declaration holds, saying who declared what and when.
// It carries the supplier's name, origin and code only. The supplier's free
// text (how it was made, the signer) never goes to a model: it is text from
// outside the account. It returns "" when no declaration holds.
func FactsBlock(fs []Fact) string {
	var lines []string
	for _, f := range fs {
		if f.Status != StatusValid {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "- %s: declared by supplier %s on %s (valid until %s) to originate in %s", f.PartName, f.Supplier, f.DeclaredOn(), f.ValidUntil, f.Origin)
		if f.HTS != "" {
			fmt.Fprintf(&b, ", tariff code %s", f.HTS)
		}
		switch {
		case f.UserOrigin == "":
		case f.Differs:
			fmt.Fprintf(&b, "; the case lists it as made in %s", f.UserOrigin)
		default:
			b.WriteString("; the case lists the same origin")
		}
		b.WriteString(".")
		lines = append(lines, b.String())
	}
	if len(lines) == 0 {
		return ""
	}
	return "Supplier declarations (a supplier's own statement, kept beside the origins the case lists):\n" + strings.Join(lines, "\n") + "\n"
}

// WithFacts appends the block to facts, keeping the one-newline ending the
// engine was benchmarked on. With no block it returns facts unchanged.
func WithFacts(facts string, fs []Fact) string {
	block := FactsBlock(fs)
	if block == "" {
		return facts
	}
	if facts != "" && !strings.HasSuffix(facts, "\n") {
		facts += "\n"
	}
	if facts != "" {
		facts += "\n"
	}
	return facts + block
}
