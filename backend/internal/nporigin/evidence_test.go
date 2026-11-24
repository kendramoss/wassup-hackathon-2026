package nporigin

import "testing"

// Every need a person can settle names what settles it; a review need never
// does, so it is never asked as a question (#770, D800).
func TestEvidenceFor(t *testing.T) {
	for _, n := range []Need{{Kind: NeedRow, Key: "row"}, {Kind: NeedShare, Key: "value_share"}, {Kind: NeedShare, Key: "weight_share"},
		{Kind: NeedFact, Key: "8542.1"}, {Kind: NeedFact, Key: "ch20.mix"}, {Kind: NeedFact, Key: "ch82.r"},
		{Kind: NeedCode, Key: "good_code"}, {Kind: NeedCode, Key: "material_hts"}, {Kind: NeedCode, Key: "material_code"}, {Kind: NeedCode, Key: "material_origin"},
		{Kind: NeedCode, Key: "country"}, {Kind: NeedCode, Key: "materials"}} {
		if EvidenceFor(n) == "" {
			t.Errorf("%+v: no evidence", n)
		}
	}
	if e := EvidenceFor(Need{Kind: NeedReview, Key: "review"}); e != "" {
		t.Errorf("review need names evidence %q", e)
	}
}
