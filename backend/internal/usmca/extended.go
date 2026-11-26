package usmca

import (
	"fmt"
	"math/big"
)

// De minimis (GN 11(e)(i)) and the parts route (GN 11(n)(i)), evaluated only
// by the extended rule set and only when the caller invokes them.

var deMinimisShare = big.NewRat(10, 100)

func isShiftKind(kind string) bool {
	switch kind {
	case KindChangeFromChapter, KindChangeFromHeading, KindChangeFromSubheading, KindChangeFromTariffItem,
		KindAllInputsFrom, KindForbiddenInputs, KindSiblingParts:
		return true
	}
	return false
}

// deMinimis re-evaluates an alternative that failed only on its tariff shift.
// The non-originating materials that do not undergo the shift may still pass
// if their value is at most 10 percent of the elected base (GN 11(e)(i)(A)).
// The alternative's other conditions (a regional value content) must still
// hold. It updates ar in place and returns the alternative's new result.
func (e *evaluator) deMinimis(ar *AlternativeResult) Tri {
	failing := map[int]bool{}
	rest := True
	shiftFailed := false
	for _, cr := range ar.Conditions {
		r := triOf(cr.Result)
		if !isShiftKind(cr.Kind) || r != False {
			rest = rest.And(r)
			continue
		}
		shiftFailed = true
		for _, m := range cr.Materials {
			if m.Result == OutcomeNotSatisfied && m.Index >= 0 {
				failing[m.Index] = true
			}
		}
	}
	if !shiftFailed || rest == False || len(failing) == 0 {
		return False
	}
	cr := ConditionResult{Kind: "de_minimis", Detail: "GN 11(e)(i): the non-originating materials that do not undergo the shift may total at most 10 percent of the elected base"}
	base, baseName := e.deMinimisBase(&cr)
	sum := new(big.Rat)
	known := true
	for i := range e.in.Materials {
		if !failing[i] {
			continue
		}
		v, ok := e.in.Materials[i].Value.Rat()
		if !ok {
			known = false
			cr.MissingFacts = append(cr.MissingFacts, MissingFact{"material_value_unknown", fmt.Sprintf("materials[%d].value", i), "Provide this material's value for de minimis."})
			continue
		}
		sum.Add(sum, v)
	}
	var t Tri
	switch {
	case base == nil || !known:
		t = Unknown3
	case base.Sign() == 0:
		t = Unknown3
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"de_minimis_base_zero", baseName, "The de minimis base must be greater than zero."})
	default:
		limit := new(big.Rat).Mul(base, deMinimisShare)
		if sum.Cmp(limit) <= 0 {
			t = True
		} else {
			t = False
		}
		share := new(big.Rat).Quo(new(big.Rat).Mul(sum, bigHundred), base)
		cr.Detail += fmt.Sprintf("; here %s percent of the %s", share.FloatString(2), baseName)
	}
	cr.Result = outcome(t)
	ar.Conditions = append(ar.Conditions, cr)
	if t == True {
		ar.Note = joinNote(ar.Note, "Tariff shift met through de minimis (GN 11(e)(i)).")
	}
	return t.And(rest)
}

func (e *evaluator) deMinimisBase(cr *ConditionResult) (*big.Rat, string) {
	var a Amount
	var name string
	switch e.in.DeMinimisBasis {
	case DeMinimisBasisTransactionValue:
		a, name = e.in.TransactionValue, "transaction_value"
	case DeMinimisBasisTotalCost:
		a, name = e.in.TotalCost, "total_cost"
	default:
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"de_minimis_basis_required", "de_minimis_basis", "Elect the de minimis base: transaction_value or total_cost."})
		return nil, ""
	}
	v, ok := a.Rat()
	if !ok {
		cr.MissingFacts = append(cr.MissingFacts, MissingFact{"de_minimis_base_unknown", name, "Provide the " + name + " of the good for de minimis."})
		return nil, name
	}
	return v, name
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

var partsRouteThresholds = func() *Thresholds {
	tv, nc := "60", "50"
	return &Thresholds{TransactionValue: &tv, NetCost: &nc}
}()

// partsRoute is GN 11(n)(i): a good produced entirely in the USMCA
// territories whose non-originating parts cannot shift because they share
// the good's classification, or because the good was imported unassembled,
// is originating with a regional value content of 60 (TV) or 50 (NC).
func (e *evaluator) partsRoute() AlternativeResult {
	ar := AlternativeResult{ID: "parts_route", Label: "GN 11(n)(i) parts route: produced entirely in USMCA territory, parts cannot shift, RVC 60 (TV) / 50 (NC)"}
	res := True

	prod := ConditionResult{Kind: "produced_in_usmca", Detail: "the good is produced entirely in the territory of one or more USMCA countries (GN 11(n)(i)(A))"}
	if e.in.Profile.ProducedInUSMCA {
		prod.Result = OutcomeSatisfied
	} else {
		prod.Result = OutcomeUnknown
		prod.MissingFacts = append(prod.MissingFacts, MissingFact{"produced_in_usmca_unconfirmed", "profile.produced_entirely_in_usmca_confirmed", "Confirm the good is produced entirely in one or more USMCA countries."})
	}
	res = res.And(triOf(prod.Result))
	ar.Conditions = append(ar.Conditions, prod)

	basis := ConditionResult{Kind: "parts_cannot_shift", Detail: "non-originating parts cannot shift (GN 11(n)(i)(B))"}
	switch e.in.Profile.PartsRouteBasis {
	case PartsBasisUnassembled:
		basis.Result = OutcomeSatisfied
		basis.Detail += ": the good was imported unassembled and classified as assembled under GRI 2(a) (caller's statement)"
	case PartsBasisSameClassification:
		basis.Detail += ": a non-originating part shares the good's subheading, or its heading when the heading has no subheadings"
		good := e.in.GoodHS
		exists := False
		for _, m := range e.in.Materials {
			var same Tri
			if m.HS == "" {
				same = Unknown3
			} else {
				same = prefixSame(good, m.HS, 6)
				if len(good) >= 6 && good[4:6] == "00" {
					same = same.Or(prefixSame(good, m.HS, 4))
				}
			}
			exists = exists.Or(counts(e.claim(m), same))
		}
		if exists == False {
			basis.Materials = append(basis.Materials, MaterialFinding{Index: -1, Result: OutcomeNotSatisfied, Note: "no non-originating part shares the good's classification"})
		}
		if exists == Unknown3 {
			basis.MissingFacts = append(basis.MissingFacts, MissingFact{"parts_classification_unknown", "materials", "Give the codes and origin of the parts that share the good's classification."})
		}
		basis.Result = outcome(exists)
	default:
		basis.Result = OutcomeUnknown
		basis.MissingFacts = append(basis.MissingFacts, MissingFact{"parts_route_basis_required", "profile.parts_route_basis", "State why the parts cannot shift: same_classification or unassembled."})
	}
	res = res.And(triOf(basis.Result))
	ar.Conditions = append(ar.Conditions, basis)

	rvc := ConditionResult{Kind: KindRVC}
	t := e.rvcCondition(&rvc, Condition{Kind: KindRVC, Thresholds: partsRouteThresholds})
	rvc.Result = outcome(t)
	res = res.And(t)
	ar.Conditions = append(ar.Conditions, rvc)

	ar.Result = outcome(res)
	return ar
}

// deemed is GN 11(p): a listed good imported from another USMCA country is
// deemed originating, notwithstanding subdivision (o). ok is false when the
// good is not listed (or its code is too short to tell).
func (e *evaluator) deemed(d *Deemed) (AlternativeResult, bool) {
	for _, g := range d.Goods {
		if hasAnyPrefix(e.in.GoodHS, g.Codes) != True {
			continue
		}
		ar := AlternativeResult{ID: "gn11p", Label: "GN 11(p): " + g.Name + " deemed originating when imported from another USMCA country"}
		cr := ConditionResult{Kind: "imported_from_usmca", Detail: "the good is imported into the United States from Mexico or Canada"}
		if e.in.Profile.ImportedFromUSMCA {
			cr.Result = OutcomeSatisfied
		} else {
			cr.Result = OutcomeUnknown
			cr.MissingFacts = append(cr.MissingFacts, MissingFact{"imported_from_usmca_unconfirmed", "profile.imported_from_usmca_country_confirmed", "Confirm the good is imported from Mexico or Canada."})
		}
		ar.Conditions = append(ar.Conditions, cr)
		ar.Result = cr.Result
		return ar, true
	}
	return AlternativeResult{}, false
}
