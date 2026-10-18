package duty

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

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
	cond    []map[string][]ConditionalExempt // per country duty: subheading → exemptions
	whole   map[string]map[string]bool       // heading → excluded subheadings
	partial map[string][]Exclusion           // subheading → described-product exclusions
}

// LoadAdditional reads the embedded additional duties.
func LoadAdditional() (*Adder, error) { return parseAdditional(additionalJSON) }

func parseAdditional(b []byte) (*Adder, error) {
	var f AdditionalFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("duty: decode additional duties: %w", err)
	}
	a := &Adder{file: f, country: map[string][]int{}, whole: map[string]map[string]bool{}, partial: map[string][]Exclusion{}}
	for _, l := range f.Lists {
		if l.Rate <= 0 || l.Rate > 1 || len(l.Subheadings) == 0 {
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
				a.whole[e.Heading] = map[string]bool{}
			}
			a.whole[e.Heading][Normalize(e.HTS)] = true
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

// For lists the additional duties on hts for origin given its MFN rate, and
// the exclusions and conditional exemptions the user should check (they are
// not applied).
func (a *Adder) For(hts, origin string, mfn float64) (extras []Extra, mayExclude []Exclusion) {
	if a == nil {
		return nil, nil
	}
	sub := Normalize(hts)
	o := domain.NormalizeCountry(origin)
	for i, l := range a.file.Lists {
		if domain.NormalizeCountry(l.Origin) != o || !a.lists[i][sub] || a.whole[l.Heading][sub] {
			continue
		}
		extras = append(extras, Extra{Name: l.Name, Heading: l.Heading, Rate: l.Rate})
	}
	if len(extras) > 0 {
		mayExclude = a.partial[sub]
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
		extras = append(extras, Extra{Name: c.Name, Heading: c.Heading, Rate: rate})
		for _, ce := range a.conditional(i, sub) {
			mayExclude = append(mayExclude, Exclusion{Heading: c.Heading, HTS: sub, Description: ce.Note + ": " + ce.Condition})
		}
	}
	sort.SliceStable(extras, func(i, j int) bool { return extras[i].Heading < extras[j].Heading })
	return extras, mayExclude
}

// Total is the MFN rate plus the extras, rounded to basis points.
func Total(mfn float64, extras []Extra) float64 {
	t := mfn
	for _, e := range extras {
		t += e.Rate
	}
	return math.Round(t*1e4) / 1e4
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
			out = append(out, domain.NormalizeCountry(m))
		}
		return out
	}
	return []string{domain.NormalizeCountry(origin)}
}
