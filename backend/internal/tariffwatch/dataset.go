package tariffwatch

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// Action is a Chapter 99 duty with dates: a Section 301 action that starts
// (or is suspended until) a day, or a Section 232 coverage. Lists in the
// embedded additional-duty file have no dates and stay in Additional.
type Action struct {
	Kind    string `json:"kind"` // KindSection301 or KindSection232
	Heading string `json:"heading"`
	Name    string `json:"name"`
	// Origin is a country, or "*" for every origin (Section 232).
	Origin string  `json:"origin"`
	Rate   float64 `json:"rate"`
	// EffectiveFrom and EffectiveThrough (YYYY-MM-DD, inclusive) bound the
	// entry dates the duty applies to; empty means no bound.
	EffectiveFrom    string   `json:"effective_from,omitempty"`
	EffectiveThrough string   `json:"effective_through,omitempty"`
	Subheadings      []string `json:"subheadings"`
	Note             string   `json:"note,omitempty"`
}

// MFNLine is one subheading's column 1 general rate as printed.
type MFNLine struct {
	HTS     string `json:"hts"`
	General string `json:"general"`
}

// Dataset is the tariff data of one HTS revision.
type Dataset struct {
	Revision string `json:"hts_revision"`
	Released string `json:"released,omitempty"`
	Source   string `json:"source"`
	// MFN lines the revision changed or carries. A line absent here keeps
	// the rate the case was priced with; the shipped data sets carry none,
	// because the MFN rate of a line is read live (D65).
	MFN []MFNLine `json:"mfn,omitempty"`
	// Additional is the embedded additional-duty data, as the duty package
	// reads it (Section 301 lists, country-wide duties, exclusions).
	Additional duty.AdditionalFile `json:"additional"`
	// Actions are the dated Chapter 99 duties and Section 232 coverage.
	Actions []Action `json:"actions,omitempty"`
}

// Embedded is the data the server prices from today: the embedded
// additional-duty file, with no dated actions.
func Embedded() (*Dataset, error) {
	f, err := duty.EmbeddedAdditional()
	if err != nil {
		return nil, err
	}
	return &Dataset{Revision: f.Revision, Source: f.Source, Additional: f}, nil
}

// Load reads a data set from JSON and checks it.
func Load(r io.Reader) (*Dataset, error) {
	var d Dataset
	dec := json.NewDecoder(io.LimitReader(r, 64<<20))
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("tariffwatch: decode data set: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// LoadFile is Load on a file.
func LoadFile(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tariffwatch: %w", err)
	}
	defer f.Close()
	d, err := Load(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// Validate checks the data set can be evaluated.
func (d *Dataset) Validate() error {
	if strings.TrimSpace(d.Revision) == "" {
		return fmt.Errorf("tariffwatch: data set has no hts_revision")
	}
	if _, err := duty.NewAdder(d.Additional); err != nil {
		return fmt.Errorf("tariffwatch: %w", err)
	}
	for _, a := range d.Actions {
		switch a.Kind {
		case KindSection301, KindSection232:
		default:
			return fmt.Errorf("tariffwatch: action %s: kind %q", a.Heading, a.Kind)
		}
		if a.Rate <= 0 || a.Rate > 5 {
			return fmt.Errorf("tariffwatch: action %s: rate %v", a.Heading, a.Rate)
		}
		if len(a.Subheadings) == 0 || strings.TrimSpace(a.Origin) == "" {
			return fmt.Errorf("tariffwatch: action %s: needs an origin and subheadings", a.Heading)
		}
		for _, s := range []string{a.EffectiveFrom, a.EffectiveThrough} {
			if s == "" {
				continue
			}
			if _, err := time.Parse(time.DateOnly, s); err != nil {
				return fmt.Errorf("tariffwatch: action %s: date %q is not YYYY-MM-DD", a.Heading, s)
			}
		}
	}
	for _, m := range d.MFN {
		if _, err := duty.ParseHTS(m.HTS); err != nil {
			return fmt.Errorf("tariffwatch: mfn line %q: %w", m.HTS, err)
		}
	}
	return nil
}

// Evaluator evaluates one data set for any number of lines and entry dates,
// building the Section 301 index once per date.
type Evaluator struct {
	ds  *Dataset
	mfn map[string]string

	mu     sync.Mutex
	adders map[string]*duty.Adder
}

// NewEvaluator prepares d for evaluation.
func NewEvaluator(d *Dataset) (*Evaluator, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	e := &Evaluator{ds: d, mfn: map[string]string{}, adders: map[string]*duty.Adder{}}
	for _, m := range d.MFN {
		if c, err := duty.ParseHTS(m.HTS); err == nil {
			e.mfn[c.Subheading] = m.General
		}
	}
	return e, nil
}

// Dataset is the data set being evaluated.
func (e *Evaluator) Dataset() *Dataset { return e.ds }

func date(s string) (time.Time, bool) {
	t, err := time.Parse(time.DateOnly, s)
	return t, err == nil
}

// active reports whether a dated action applies on the UTC calendar day of
// asOf.
func (a Action) active(asOf time.Time) bool {
	y, m, d := asOf.UTC().Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if t, ok := date(a.EffectiveFrom); ok && day.Before(t) {
		return false
	}
	if t, ok := date(a.EffectiveThrough); ok && day.After(t) {
		return false
	}
	return true
}

func (a Action) covers(sub, origin string) bool {
	if a.Origin != "*" && domain.DutyCountry(a.Origin) != domain.DutyCountry(origin) {
		return false
	}
	for _, s := range a.Subheadings {
		if duty.Normalize(s) == sub {
			return true
		}
	}
	return false
}

// adder returns the Section 301 index for asOf: the undated lists plus the
// Section 301 actions in force that day.
func (e *Evaluator) adder(asOf time.Time) (*duty.Adder, error) {
	key := asOf.UTC().Format(time.DateOnly)
	e.mu.Lock()
	defer e.mu.Unlock()
	if a, ok := e.adders[key]; ok {
		return a, nil
	}
	f := e.ds.Additional
	f.Lists = append([]duty.AdditionalList(nil), f.Lists...)
	for _, act := range e.ds.Actions {
		if act.Kind != KindSection301 || !act.active(asOf) {
			continue
		}
		f.Lists = append(f.Lists, duty.AdditionalList{Heading: act.Heading, Name: act.Name, Origin: act.Origin, Rate: act.Rate, Note: act.Note, Subheadings: act.Subheadings})
	}
	a, err := duty.NewAdder(f)
	if err != nil {
		return nil, err
	}
	e.adders[key] = a
	return a, nil
}

// Evaluate prices one line for one origin on the entry day asOf from the
// data set. mfn is the MFN rate to use when the data set carries no line of
// its own for hts (nil when unknown: the MFN part is then left out, and no
// MFN change is ever reported).
func (e *Evaluator) Evaluate(hts, origin string, mfn *float64, asOf time.Time) (OriginBasis, error) {
	code, err := duty.ParseHTS(hts)
	if err != nil {
		return OriginBasis{}, err
	}
	sub := code.Subheading
	ob := OriginBasis{Origin: origin, Source: "dataset"}
	if g, ok := e.mfn[sub]; ok {
		if r, ok := duty.ParseAdValorem(g); ok {
			mfn = &r
		}
	}
	m := 0.0
	if mfn != nil {
		m = *mfn
		v := *mfn
		ob.MFN = &v
		ob.Components = append(ob.Components, Component{Kind: KindMFN, Name: "MFN (column 1 general)", Heading: sub, Rate: v})
	}
	ad, err := e.adder(asOf)
	if err != nil {
		return ob, err
	}
	res := ad.ForAsOf(sub, origin, m, asOf)
	dated := map[string]Action{}
	for _, a := range e.ds.Actions {
		dated[a.Heading] = a
	}
	for _, x := range res.Extras {
		c := Component{Kind: KindSection301, Name: x.Name, Heading: x.Heading, Rate: x.Rate}
		// Dates belong to the component only when it came from an action in
		// force; an undated list that shares the heading keeps no dates.
		if a, ok := dated[x.Heading]; ok && a.Kind == KindSection301 && a.active(asOf) {
			c.From, c.Through = a.EffectiveFrom, a.EffectiveThrough
		}
		ob.Components = append(ob.Components, c)
	}
	for _, a := range e.ds.Actions {
		if a.Kind == KindSection232 && a.active(asOf) && a.covers(sub, origin) {
			ob.Components = append(ob.Components, Component{Kind: KindSection232, Name: a.Name, Heading: a.Heading, Rate: a.Rate, From: a.EffectiveFrom, Through: a.EffectiveThrough})
		}
	}
	for _, x := range res.MayExclude {
		ob.Exclusions = append(ob.Exclusions, Exclusion{Heading: x.Heading, HTS: x.HTS, EndDate: x.EndDate, Status: x.Status})
	}
	for _, x := range res.Expired {
		ob.Exclusions = append(ob.Exclusions, Exclusion{Heading: x.Heading, HTS: x.HTS, EndDate: x.EndDate, Status: x.Status})
	}
	sort.SliceStable(ob.Components, func(i, j int) bool { return ob.Components[i].Heading < ob.Components[j].Heading })
	sort.SliceStable(ob.Exclusions, func(i, j int) bool {
		if ob.Exclusions[i].Heading != ob.Exclusions[j].Heading {
			return ob.Exclusions[i].Heading < ob.Exclusions[j].Heading
		}
		return ob.Exclusions[i].HTS < ob.Exclusions[j].HTS
	})
	ob.Rate = total(ob.Components)
	return ob, nil
}

// Covers232 reports whether the data set lists Section 232 coverage for hts
// and origin on asOf.
func (e *Evaluator) Covers232(hts, origin string, asOf time.Time) bool {
	code, err := duty.ParseHTS(hts)
	if err != nil {
		return false
	}
	for _, a := range e.ds.Actions {
		if a.Kind == KindSection232 && a.active(asOf) && a.covers(code.Subheading, origin) {
			return true
		}
	}
	return false
}

func total(cs []Component) float64 {
	t := 0.0
	for _, c := range cs {
		t += c.Rate
	}
	return math.Round(t*1e4) / 1e4
}

// MFNFromHTSJSON reads the column 1 general rates out of a USITC HTS JSON
// export (the file the HTS site's export function writes: a list of rows
// with htsno and general). Only 8-digit subheadings that print a general
// rate are kept; chapters 98 and 99 are left out.
func MFNFromHTSJSON(r io.Reader) ([]MFNLine, error) {
	var rows []struct {
		HTSNo   string `json:"htsno"`
		General string `json:"general"`
	}
	if err := json.NewDecoder(io.LimitReader(r, 128<<20)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("tariffwatch: decode HTS export: %w", err)
	}
	var out []MFNLine
	for _, row := range rows {
		if strings.TrimSpace(row.General) == "" || strings.HasPrefix(row.HTSNo, "98") || strings.HasPrefix(row.HTSNo, "99") {
			continue
		}
		c, err := duty.ParseHTS(row.HTSNo)
		if err != nil || c.Statistical != "" || len(strings.ReplaceAll(row.HTSNo, ".", "")) != 8 {
			continue
		}
		out = append(out, MFNLine{HTS: c.Subheading, General: strings.TrimSpace(row.General)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HTS < out[j].HTS })
	return out, nil
}
