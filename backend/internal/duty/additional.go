package duty

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Additional duties by origin and subheading, from HTS Chapter 99 (D90):
// the China Section 301 lists (U.S. note 20: headings 9903.88.01, .02, .03,
// .04 and .15) and the 2026 country-wide Section 301 duties (headings
// 9903.05.xx, 91 FR 47318) less the products U.S. note 52 exempts. The
// lists are parsed from the HTS PDF by scripts/duty301 and embedded, so a
// lookup makes no network call. A product exclusion that names a
// described product rather than a whole subheading is not applied: it is
// reported as "may apply" so the user can check it.
//
//go:embed additional.json
var additionalJSON []byte

// AdditionalFile is the embedded data's shape.
type AdditionalFile struct {
	Source   string `json:"source"`
	Revision string `json:"hts_revision"`
	Checked  string `json:"checked"`
	// Lists are subheading lists that add a duty for one origin.
	Lists []AdditionalList `json:"lists"`
	// Country is a duty on every product of an origin, except Exempt.
	Country []CountryDuty `json:"country"`
	// Exclusions are product exclusions from Lists; WholeSubheading ones
	// remove the duty, the rest are only reported.
	Exclusions []Exclusion `json:"exclusions"`
}

// AdditionalList is one Chapter 99 heading's subheading list.
type AdditionalList struct {
	Heading     string   `json:"heading"`
	Name        string   `json:"name"`
	Origin      string   `json:"origin"`
	Rate        float64  `json:"rate"`
	Note        string   `json:"note"`
	Subheadings []string `json:"subheadings"`
}

// CountryDuty is a duty on every product of an origin, except the exempt
// subheadings. Mode "additive" adds Rate; "floor_total" (EU, Japan, South
// Korea, Switzerland, Taiwan; U.S. note 52(k)) raises the MFN rate to Rate
// when it is lower and adds nothing otherwise.
type CountryDuty struct {
	Heading string   `json:"heading"`
	Name    string   `json:"name"`
	Origin  string   `json:"origin"`
	Rate    float64  `json:"rate"`
	Mode    string   `json:"mode"`
	Source  string   `json:"source"`
	Exempt  []string `json:"exempt"`
	// Conditional exemptions depend on the use or the entry claim (civil
	// aircraft, pharmaceutical use, a trade agreement claim, a described
	// product); they are reported, not applied.
	Conditional []ConditionalExempt `json:"conditional,omitempty"`
}

// ConditionalExempt is a U.S. note 52 exemption that applies only under a
// condition the planner cannot check.
type ConditionalExempt struct {
	Note      string   `json:"note"`
	Condition string   `json:"condition"`
	HTS       []string `json:"hts"`
}

// Exclusion is a Section 301 product exclusion.
type Exclusion struct {
	Heading         string `json:"heading"`
	HTS             string `json:"hts"`
	WholeSubheading bool   `json:"whole_subheading"`
	Description     string `json:"description"`
	EndDate         string `json:"end_date"`
	// Status is set on output only: "" (valid on the as-of date), "expired"
	// (its stored end_date is before the as-of date; historical only) or
	// "review" (no readable end_date or as-of date; confirm before relying).
	Status string `json:"status,omitempty"`
}

// MaxListRate bounds a list's additional ad valorem rate (a fraction: 5 is
// 500 percent). It matches the bound tariffwatch.Dataset.Validate puts on a
// dated action, whose rate becomes a list rate; a rate above 100 percent is a
// legal ad valorem rate, and a typo such as 25 for 0.25 is still refused.
const MaxListRate = 5

// Exclusion statuses.
const (
	StatusExpired = "expired"
	StatusReview  = "review"
)

// exclusionEndLayout is the stored end_date format (additional.json).
const exclusionEndLayout = "2006-01-02"

// exclusionStatus classifies e on the UTC calendar date of asOf. An exclusion
// is valid through its end_date (D90: "until 9 Nov 2026"), so the end date
// itself is still valid. An unreadable end_date or a zero asOf is "review".
func exclusionStatus(e Exclusion, asOf time.Time) string {
	end, err := time.Parse(exclusionEndLayout, e.EndDate)
	if err != nil || asOf.IsZero() {
		return StatusReview
	}
	y, m, d := asOf.UTC().Date()
	if time.Date(y, m, d, 0, 0, 0, 0, time.UTC).After(end) {
		return StatusExpired
	}
	return ""
}

// AsOfResult is the outcome of ForAsOf.
type AsOfResult struct {
	Extras []Extra
	// MayExclude are exclusions valid on the date, plus "review" ones;
	// they are reported, never applied.
	MayExclude []Exclusion
	// Expired are stored exclusions whose end date had passed: historical,
	// not applicable now. Nothing is said about whether they were extended.
	Expired []Exclusion
}

// Extra is one additional duty that applies.
type Extra struct {
	Name    string  `json:"name"`
	Heading string  `json:"heading"`
	Rate    float64 `json:"rate"`
}

// Adder looks up additional duties.
type Adder struct {
	file    AdditionalFile
	lists   []map[string]bool
	country map[string][]int // origin → indices into file.Country
	exempt  []map[string]bool
	cond    []map[string][]ConditionalExempt  // per country duty: subheading → exemptions
	whole   map[string]map[string][]Exclusion // heading → subheading → whole-subheading exclusions
	now     func() time.Time                  // default as-of clock for For; nil = no expiry filtering
	partial map[string][]Exclusion            // subheading → described-product exclusions
}

// LoadAdditional reads the embedded additional duties.
// Its For method uses the real clock; use WithClock to inject another.
func LoadAdditional() (*Adder, error) {
	a, err := parseAdditional(additionalJSON)
	if err != nil {
		return nil, err
	}
	a.now = time.Now
	return a, nil
}

// WithClock returns a copy whose For method uses now as the as-of date.
func (a *Adder) WithClock(now func() time.Time) *Adder {
	if a == nil {
		return nil
	}
	c := *a
	c.now = now
	return &c
}

func parseAdditional(b []byte) (*Adder, error) {
	var f AdditionalFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("duty: decode additional duties: %w", err)
	}
	return NewAdder(f)
}

// EmbeddedAdditional returns the embedded additional-duty data as decoded
// (the basis every saved case is priced on, #917).
func EmbeddedAdditional() (AdditionalFile, error) {
	var f AdditionalFile
	if err := json.Unmarshal(additionalJSON, &f); err != nil {
		return f, fmt.Errorf("duty: decode additional duties: %w", err)
	}
	return f, nil
}

// NewAdder validates f and builds an Adder over it, with no clock (nothing
// filtered by date unless ForAsOf is used). The tariff watch builds one per
// HTS revision data set it compares (#917).
func NewAdder(f AdditionalFile) (*Adder, error) {
	a := &Adder{file: f, country: map[string][]int{}, whole: map[string]map[string][]Exclusion{}, partial: map[string][]Exclusion{}}
	for _, l := range f.Lists {
		if l.Rate <= 0 || l.Rate > MaxListRate || len(l.Subheadings) == 0 {
			return nil, fmt.Errorf("duty: list %s: rate %v, %d subheadings", l.Heading, l.Rate, len(l.Subheadings))
		}
		set := map[string]bool{}
		for _, s := range l.Subheadings {
			set[Normalize(s)] = true
		}
		a.lists = append(a.lists, set)
	}
	for i, c := range f.Country {
		if c.Mode != "" && c.Mode != "additive" && c.Mode != "floor_total" {
			return nil, fmt.Errorf("duty: country duty %s: mode %q", c.Heading, c.Mode)
		}
		if c.Rate <= 0 || c.Rate > 1 {
			return nil, fmt.Errorf("duty: country duty %s: rate %v", c.Heading, c.Rate)
		}
		for _, o := range originsOf(c.Origin) {
			a.country[o] = append(a.country[o], i)
		}
		set := map[string]bool{}
		for _, s := range c.Exempt {
			set[exemptKey(s)] = true
		}
		a.exempt = append(a.exempt, set)
		cm := map[string][]ConditionalExempt{}
		for _, ce := range c.Conditional {
			for _, h := range ce.HTS {
				k := exemptKey(h)
				cm[k] = append(cm[k], ce)
			}
		}
		a.cond = append(a.cond, cm)
	}
	for _, e := range f.Exclusions {
		if e.WholeSubheading {
			if a.whole[e.Heading] == nil {
				a.whole[e.Heading] = map[string][]Exclusion{}
			}
			k := Normalize(e.HTS)
			a.whole[e.Heading][k] = append(a.whole[e.Heading][k], e)
			continue
		}
		a.partial[Normalize(e.HTS)] = append(a.partial[Normalize(e.HTS)], e)
	}
	return a, nil
}

// exemptKey is how an exempt number is matched: 8-digit subheadings, or the
// 6-digit heading when the note lists one.
func exemptKey(hts string) string {
	n := Normalize(hts)
	if len(n) < 10 { // fewer than 8 digits
		return strings.ReplaceAll(n, ".", "")
	}
	return n
}

// keys are the forms a note may list sub under: the 8-digit subheading,
// then its 6- and 4-digit headings.
func keys(sub string) []string {
	digits := strings.ReplaceAll(sub, ".", "")
	out := []string{sub}
	for n := 6; n >= 4; n -= 2 {
		if len(digits) >= n {
			out = append(out, digits[:n])
		}
	}
	return out
}

func (a *Adder) exempted(i int, sub string) bool {
	for _, k := range keys(sub) {
		if a.exempt[i][k] {
			return true
		}
	}
	return false
}

func (a *Adder) conditional(i int, sub string) []ConditionalExempt {
	var out []ConditionalExempt
	for _, k := range keys(sub) {
		out = append(out, a.cond[i][k]...)
	}
	return out
}

// For is ForAsOf at the Adder's clock; without one nothing is filtered by
// date (every stored exclusion is reported, as before) and expired ones are
// not separated out. Prefer ForAsOf with the request's entry date.
func (a *Adder) For(hts, origin string, mfn float64) (extras []Extra, mayExclude []Exclusion) {
	if a == nil {
		return nil, nil
	}
	if a.now == nil {
		r := a.lookup(hts, origin, mfn, time.Time{}, false)
		return r.Extras, r.MayExclude
	}
	r := a.ForAsOf(hts, origin, mfn, a.now())
	return r.Extras, r.MayExclude
}

// ForAsOf lists the additional duties on hts for origin given its MFN rate,
// as of the date asOf (its UTC calendar date), and the exclusions and
// conditional exemptions the user should check (they are not applied).
// Stored exclusions past their end date are returned in Expired, never in
// MayExclude. A whole-subheading exclusion removes the list duty only while
// valid; once it has lapsed the stored duty is listed again and the
// exclusion appears in Expired, without any statement on extensions.
func (a *Adder) ForAsOf(hts, origin string, mfn float64, asOf time.Time) AsOfResult {
	if a == nil {
		return AsOfResult{}
	}
	return a.lookup(hts, origin, mfn, asOf, true)
}

func (a *Adder) lookup(hts, origin string, mfn float64, asOf time.Time, dated bool) AsOfResult {
	var res AsOfResult
	sub := Normalize(hts)
	o := domain.DutyCountry(origin)
	status := func(e Exclusion) string {
		if !dated {
			return ""
		}
		return exclusionStatus(e, asOf)
	}
	for i, l := range a.file.Lists {
		if domain.DutyCountry(l.Origin) != o || !a.lists[i][sub] {
			continue
		}
		removed := false
		for _, e := range a.whole[l.Heading][sub] {
			switch st := status(e); st {
			case "":
				removed = true
			case StatusExpired:
				e.Status = st
				res.Expired = append(res.Expired, e)
			default: // unreadable date: keep the duty and ask for review
				e.Status = st
				e.Description = reviewText(e.Description)
				res.MayExclude = append(res.MayExclude, e)
			}
		}
		if removed {
			continue
		}
		res.Extras = append(res.Extras, Extra{Name: l.Name, Heading: l.Heading, Rate: l.Rate})
	}
	if len(res.Extras) > 0 {
		for _, e := range a.partial[sub] {
			switch st := status(e); st {
			case "":
				res.MayExclude = append(res.MayExclude, e)
			case StatusExpired:
				e.Status = st
				res.Expired = append(res.Expired, e)
			default:
				e.Status = st
				e.Description = reviewText(e.Description)
				res.MayExclude = append(res.MayExclude, e)
			}
		}
	}
	for _, i := range a.country[o] {
		if a.exempted(i, sub) {
			continue
		}
		c := a.file.Country[i]
		rate := c.Rate
		if c.Mode == "floor_total" {
			rate = math.Round(max(c.Rate-mfn, 0)*1e4) / 1e4 // tops the total up to Rate
		}
		if rate <= 0 {
			continue
		}
		res.Extras = append(res.Extras, Extra{Name: c.Name, Heading: c.Heading, Rate: rate})
		for _, ce := range a.conditional(i, sub) {
			res.MayExclude = append(res.MayExclude, Exclusion{Heading: c.Heading, HTS: sub, Description: ce.Note + ": " + ce.Condition})
		}
	}
	sort.SliceStable(res.Extras, func(i, j int) bool { return res.Extras[i].Heading < res.Extras[j].Heading })
	return res
}

func reviewText(d string) string {
	return d + " (end date unreadable or no as-of date: confirm this exclusion's dates before relying on it)"
}

// Total is the MFN rate plus the extras, rounded to basis points.
func Total(mfn float64, extras []Extra) float64 {
	t := mfn
	for _, e := range extras {
		t += e.Rate
	}
	return math.Round(t*1e4) / 1e4
}

// Revision is the HTS revision the data was read from.
func (a *Adder) Revision() string {
	if a == nil {
		return ""
	}
	return a.file.Revision
}

// Source describes the embedded data for notes.
func (a *Adder) Source() string {
	return fmt.Sprintf("%s, %s, parsed %s", a.file.Source, a.file.Revision, a.file.Checked)
}

// euMembers are the 27 member states; the HTS names the European Union as
// one origin (heading 9903.05.39), and users name the country.
var euMembers = []string{"Austria", "Belgium", "Bulgaria", "Croatia", "Cyprus", "Czech Republic", "Denmark", "Estonia", "Finland",
	"France", "Germany", "Greece", "Hungary", "Ireland", "Italy", "Latvia", "Lithuania", "Luxembourg", "Malta", "Netherlands",
	"Poland", "Portugal", "Romania", "Slovakia", "Slovenia", "Spain", "Sweden"}

// originsOf lists the normalised origins a country duty applies to.
func originsOf(origin string) []string {
	if origin == "European Union" {
		out := []string{"European Union"}
		for _, m := range euMembers {
			out = append(out, domain.DutyCountry(m))
		}
		return out
	}
	return []string{domain.DutyCountry(origin)}
}
