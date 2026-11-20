package opcost

import "testing"

func money() MoneyBasis {
	return MoneyBasis{Illustrative: true, AnnualImportValueUSD: 1_000_000,
		DutyRates:  map[string]float64{"china": 0.25, "thailand": 0.05},
		GroupCosts: map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}},
		Operations: costly(5).Ops}
}

func TestBasisHashStableAndSensitive(t *testing.T) {
	base := BasisHash(money())
	if base != BasisHash(money()) {
		t.Fatal("hash must be stable across map order")
	}
	// provenance text is not an input
	m := money()
	m.Operations.Note = "changed"
	p := m.Operations.Groups["fa"].Plants["p2"]
	p.Sources = map[string]string{"capex_usd": "quote"}
	m.Operations.Groups["fa"].Plants["p2"] = p
	if BasisHash(m) != base {
		t.Fatal("note and sources must not change the basis")
	}
	// any numeric, basis, ref, unit or constraint change does
	muts := map[string]func(*MoneyBasis){
		"value":   func(m *MoneyBasis) { m.AnnualImportValueUSD++ },
		"rate":    func(m *MoneyBasis) { m.DutyRates["china"] = 0.26 },
		"group":   func(m *MoneyBasis) { m.GroupCosts["fa"]["p2"]++ },
		"horizon": func(m *MoneyBasis) { m.Operations.HorizonYears = f(6) },
		"lead":    func(m *MoneyBasis) { m.Operations.MaxLeadTimeDays = n(1) },
		"basis": func(m *MoneyBasis) {
			g := m.Operations.Groups["fa"]
			g.CostBasis = BasisIncluded
			m.Operations.Groups["fa"] = g
		},
		"unit": func(m *MoneyBasis) { g := m.Operations.Groups["fa"]; g.Unit = "pcs"; m.Operations.Groups["fa"] = g },
		"capex": func(m *MoneyBasis) {
			p := m.Operations.Groups["fa"].Plants["p2"]
			p.Capex = f(1)
			m.Operations.Groups["fa"].Plants["p2"] = p
		},
		"zero": func(m *MoneyBasis) {
			p := m.Operations.Groups["fa"].Plants["p1"]
			p.Capex = f(0)
			m.Operations.Groups["fa"].Plants["p1"] = p
		},
		"ref": func(m *MoneyBasis) {
			p := m.Operations.Groups["fa"].Plants["p2"]
			p.Ref = &rp3
			m.Operations.Groups["fa"].Plants["p2"] = p
		},
		"moq": func(m *MoneyBasis) {
			p := m.Operations.Groups["fa"].Plants["p2"]
			p.MOQUnits = f(5)
			m.Operations.Groups["fa"].Plants["p2"] = p
		},
		"lead day": func(m *MoneyBasis) {
			p := m.Operations.Groups["fa"].Plants["p2"]
			p.LeadTimeDays = n(9)
			m.Operations.Groups["fa"].Plants["p2"] = p
		},
	}
	for name, mut := range muts {
		m := money()
		mut(&m)
		if BasisHash(m) == base {
			t.Errorf("%s change must change the basis", name)
		}
	}
}

func TestBasisLegacyCaseHasNoOperationsPart(t *testing.T) {
	for _, o := range []*Operations{nil, {}, {Groups: map[string]GroupOps{}}} {
		if CanonicalOps(o) != nil {
			t.Fatal("inactive operations must contribute nothing")
		}
		m := money()
		m.Operations = o
		n := money()
		n.Operations = nil
		if BasisHash(m) != BasisHash(n) {
			t.Fatal("inactive operations must hash as absent")
		}
	}
	// empty maps are the same as absent ones
	a, b := money(), money()
	a.Operations, b.Operations = nil, nil
	b.DutyRates, b.GroupCosts = map[string]float64{"china": 0.25, "thailand": 0.05}, map[string]map[string]float64{"fa": {"p1": 300_000, "p2": 320_000}, "x": {}}
	if BasisHash(a) != BasisHash(b) {
		t.Fatal("an empty group-cost row is not an input")
	}
}

// The wiring contract: the stamp is taken from the RAW economics before the
// duty table or the live lookup fills rates. Hashing a priced copy would differ
// and mark every stored plan stale.
func TestBasisFlowRawThenPriced(t *testing.T) {
	raw := money()
	raw.DutyRates = map[string]float64{"china": 0.25} // the user left thailand blank
	stamp := BasisHash(raw)
	priced := money() // the filled-in copy the planner runs on
	priced.DutyRates = map[string]float64{"china": 0.25, "thailand": 0.05}
	if BasisHash(priced) == stamp {
		t.Fatal("test premise: filled rates differ from raw")
	}
	// the stored case is the raw economics: a fresh hash is unchanged
	stored := money()
	stored.DutyRates = map[string]float64{"china": 0.25}
	if BasisHash(stored) != stamp {
		t.Fatal("fresh basis of the stored raw economics must equal the stamp")
	}
	// a later cost change gives a different basis
	stored.Operations.Groups["fa"].Plants["p2"] = func() PlantOps { p := stored.Operations.Groups["fa"].Plants["p2"]; p.Capex = f(1); return p }()
	if BasisHash(stored) == stamp {
		t.Fatal("a cost change must change the basis")
	}
}
