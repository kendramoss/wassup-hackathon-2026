package tariffwatch

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func rp(r float64) *float64 { v := math.Round(r*1e4) / 1e4; return &v }

func pct(r float64) string { return fmt.Sprintf("%.4g%%", math.Round(r*1e6)/1e4) }

// compared reports whether a component takes part in a comparison: only the
// duties the data set models (MFN, Section 301, and Section 232 when the
// snapshot modelled it). A component of another kind, such as one hand-entered
// in the researched table, is left alone, never reported as removed.
func compared(c Component, model232 bool) bool {
	switch c.Kind {
	case KindMFN, KindSection301:
		return true
	case KindSection232:
		return model232
	}
	return false
}

func key(c Component) string {
	if c.Kind == KindMFN {
		return KindMFN
	}
	return c.Kind + ":" + c.Heading
}

// CompareOrigin lists what differs between the duties an origin was priced
// at (old) and the same line evaluated on a newer data set (new). Changes
// that move the rate are listed first, in heading order.
func CompareOrigin(old, new OriginBasis, model232 bool) []Change {
	oc, nc := map[string]Component{}, map[string]Component{}
	for _, c := range old.Components {
		if compared(c, model232) {
			oc[key(c)] = c
		}
	}
	for _, c := range new.Components {
		if compared(c, model232) {
			nc[key(c)] = c
		}
	}
	// The data set may carry no MFN rate for the line; then the MFN part is
	// not a comparison, not a removal.
	if _, ok := nc[KindMFN]; !ok {
		delete(oc, KindMFN)
	}
	var out []Change
	keys := map[string]bool{}
	for k := range oc {
		keys[k] = true
	}
	for k := range nc {
		keys[k] = true
	}
	order := make([]string, 0, len(keys))
	for k := range keys {
		order = append(order, k)
	}
	sort.Strings(order)
	for _, k := range order {
		o, hadOld := oc[k]
		n, hasNew := nc[k]
		switch {
		case hadOld && !hasNew:
			out = append(out, Change{Origin: old.Origin, Kind: ChangeRemoved, Heading: o.Heading, Name: o.Name, OldRate: rp(o.Rate), NewRate: rp(0), MovesRate: true,
				Old: pct(o.Rate), New: "not in force", Note: dateNote(o)})
		case !hadOld && hasNew:
			out = append(out, Change{Origin: old.Origin, Kind: ChangeAdded, Heading: n.Heading, Name: n.Name, OldRate: rp(0), NewRate: rp(n.Rate), MovesRate: true,
				Old: "none", New: pct(n.Rate), Note: dateNote(n)})
		default:
			if math.Abs(o.Rate-n.Rate) > 1e-9 {
				kind := ChangeRate
				if k == KindMFN {
					kind = ChangeMFN
				}
				out = append(out, Change{Origin: old.Origin, Kind: kind, Heading: n.Heading, Name: n.Name, OldRate: rp(o.Rate), NewRate: rp(n.Rate), MovesRate: true, Old: pct(o.Rate), New: pct(n.Rate)})
			}
			if o.From != n.From || o.Through != n.Through {
				out = append(out, Change{Origin: old.Origin, Kind: ChangeWindow, Heading: n.Heading, Name: n.Name, Old: window(o.From, o.Through), New: window(n.From, n.Through),
					Note: "the dates the duty applies to changed; the rate for the entry date checked is unchanged"})
			}
		}
	}
	oe, ne := map[string]Exclusion{}, map[string]Exclusion{}
	for _, e := range old.Exclusions {
		oe[e.Heading+"|"+e.HTS] = e
	}
	for _, e := range new.Exclusions {
		ne[e.Heading+"|"+e.HTS] = e
	}
	ek := make([]string, 0)
	seen := map[string]bool{}
	for k := range oe {
		seen[k] = true
		ek = append(ek, k)
	}
	for k := range ne {
		if !seen[k] {
			ek = append(ek, k)
		}
	}
	sort.Strings(ek)
	for _, k := range ek {
		o, hadOld := oe[k]
		n, hasNew := ne[k]
		note := "a described-product exclusion is reported, never applied to the rate: check that the product matches it"
		switch {
		case hadOld && !hasNew:
			out = append(out, Change{Origin: old.Origin, Kind: ChangeExclusionGone, Heading: o.Heading, Name: o.HTS, Old: "through " + o.EndDate, New: "not listed", Note: note})
		case !hadOld && hasNew:
			out = append(out, Change{Origin: old.Origin, Kind: ChangeExclusionAdded, Heading: n.Heading, Name: n.HTS, Old: "not listed", New: "through " + n.EndDate, Note: note})
		case o.EndDate != n.EndDate:
			out = append(out, Change{Origin: old.Origin, Kind: ChangeExclusionWindow, Heading: n.Heading, Name: n.HTS, Old: "through " + o.EndDate, New: "through " + n.EndDate, Note: note})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MovesRate && !out[j].MovesRate })
	return out
}

func window(from, through string) string {
	switch {
	case from == "" && through == "":
		return "no dates"
	case through == "":
		return "from " + from
	case from == "":
		return "through " + through
	}
	return from + " to " + through
}

func dateNote(c Component) string {
	if c.From == "" && c.Through == "" {
		return ""
	}
	return "applies " + window(c.From, c.Through)
}

// PlanInput is the best shown plan of a case.
type PlanInput struct {
	To             string  // the origin the plan moves to
	DutyEffectYear float64 // the plan's duty_effect_per_year as stored
}

// CaseInput is what Watch needs of a case.
type CaseInput struct {
	Basis Basis
	// BaseOrigin is the origin of the current setup.
	BaseOrigin string
	Plan       *PlanInput
}

func (b Basis) rateOf(origin string) (OriginBasis, bool) {
	want := domain.DutyCountry(origin)
	for _, o := range b.Origins {
		if domain.DutyCountry(o.Origin) == want {
			return o, true
		}
	}
	return OriginBasis{}, false
}

// Watch compares the basis a case was priced on with the data set, for the
// entry day asOf, and works out the change in its annual duty figure and in
// its plan's duty effect.
func (e *Evaluator) Watch(in CaseInput, asOf, now time.Time) (Result, error) {
	b := in.Basis
	res := Result{CheckedAt: now.UTC(), DatasetRevision: e.ds.Revision, BasisRevision: b.HTSRevision, AsOf: asOf.UTC().Format(time.DateOnly), Changes: []Change{}}
	delta := map[string]float64{}
	for _, o := range b.Origins {
		if o.Source == "user" || len(o.Components) == 0 {
			res.Notes = append(res.Notes, fmt.Sprintf("%s: the rate was entered by the user, so there is nothing to compare it with", o.Origin))
			continue
		}
		nb, err := e.Evaluate(b.HTS, o.Origin, o.MFN, asOf)
		if err != nil {
			return res, err
		}
		changes := CompareOrigin(o, nb, b.Section232.Modelled)
		d := 0.0
		for _, c := range changes {
			if c.MovesRate {
				d += *c.NewRate - *c.OldRate
			}
		}
		d = math.Round(d*1e4) / 1e4
		res.Changes = append(res.Changes, changes...)
		if !b.Section232.Modelled && e.Covers232(b.HTS, o.Origin, asOf) {
			res.Changes = append(res.Changes, Change{Origin: o.Origin, Kind: Change232, Name: "Section 232",
				Old: "not modelled", New: "listed in the newer data set",
				Note: "the rate this case was priced at does not include Section 232 and the newer data set lists this line: check it before relying on the duty figure"})
		}
		if d != 0 {
			delta[domain.DutyCountry(o.Origin)] = d
			res.Rates = append(res.Rates, RateDelta{Origin: o.Origin, OldRate: o.Rate, NewRate: math.Round((o.Rate+d)*1e4) / 1e4, Delta: d})
		}
	}
	res.Affected = len(res.Changes) > 0
	v := b.AnnualImportUSD
	if v <= 0 {
		res.Notes = append(res.Notes, "no annual import value on the case, so no dollar change is worked out")
		return res, nil
	}
	if base, ok := b.rateOf(in.BaseOrigin); ok {
		d := delta[domain.DutyCountry(in.BaseOrigin)]
		before := math.Round(v*base.Rate*100) / 100
		res.Annual = &AnnualDuty{Origin: in.BaseOrigin, Before: before, After: math.Round(v*(base.Rate+d)*100) / 100}
		res.Annual.Change = math.Round((res.Annual.After-res.Annual.Before)*100) / 100
	}
	if p := in.Plan; p != nil {
		_, okFrom := b.rateOf(in.BaseOrigin)
		_, okTo := b.rateOf(p.To)
		if okFrom && okTo {
			df, dt := delta[domain.DutyCountry(in.BaseOrigin)], delta[domain.DutyCountry(p.To)]
			chg := math.Round(v*(df-dt)*100) / 100
			res.Plan = &PlanDuty{From: in.BaseOrigin, To: p.To, Before: p.DutyEffectYear, After: math.Round((p.DutyEffectYear+chg)*100) / 100, Change: chg}
		} else {
			res.Notes = append(res.Notes, "the plan's duty effect is not rechecked: a rate for the current origin or the plan's origin is missing")
		}
	}
	return res, nil
}
