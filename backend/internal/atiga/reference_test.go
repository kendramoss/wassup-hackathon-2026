package atiga

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestReferenceCases runs the authority worked examples pre-registered in
// D927 (eval/atiga/reference-cases.json) and checks each stated result.
func TestReferenceCases(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "eval", "atiga", "reference-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct {
			ID     string `json:"id"`
			Input  Input  `json:"input"`
			Expect struct {
				Outcome      string            `json:"outcome"`
				Applied      string            `json:"applied"`
				Alternative  map[string]string `json:"alternative"`
				Accumulation *bool             `json:"accumulation"`
				IndirectMin  *float64          `json:"rvc_indirect_min"`
				IndirectMax  *float64          `json:"rvc_indirect_max"`
				DirectMin    *float64          `json:"rvc_direct_min"`
				DirectMax    *float64          `json:"rvc_direct_max"`
			} `json:"expect"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) != 7 {
		t.Fatalf("%d cases, want the 7 pre-registered", len(doc.Cases))
	}
	// knownDeviation lists pre-registered checks that failed, with the
	// cause, as reported in D927. Nothing else may fail.
	knownDeviation := map[string]string{
		"sgc-b-rvc": "the source prints RVC 36 %; (15 - 9.50) / 15 and (1 + 2 + 2.50) / 15 are 36.67 %, so the pre-registered 35.9-36.1 range copied the source's rounding down; the stated result, RVC40 not met, matches",
	}
	inRange := func(p, lo, hi *float64) bool {
		if lo == nil {
			return true
		}
		return p != nil && *p >= *lo && *p <= *hi
	}
	for _, c := range doc.Cases {
		res, err := Check(c.Input)
		if err != nil {
			t.Errorf("%s: %v", c.ID, err)
			continue
		}
		e := c.Expect
		if e.Outcome != "" && res.Outcome != e.Outcome {
			t.Errorf("%s: outcome %s, want %s; %v", c.ID, res.Outcome, e.Outcome, res.Reasons)
		}
		if e.Applied != "" && res.Applied != e.Applied {
			t.Errorf("%s: applied %q, want %q", c.ID, res.Applied, e.Applied)
		}
		for rule, want := range e.Alternative {
			got := ""
			for _, a := range res.Alternatives {
				if a.Rule == rule {
					got = a.Outcome
				}
			}
			if got != want {
				t.Errorf("%s: alternative %s = %q, want %q", c.ID, rule, got, want)
			}
		}
		if e.Accumulation != nil && res.Accumulation != *e.Accumulation {
			t.Errorf("%s: accumulation %v", c.ID, res.Accumulation)
		}
		var ind, dir *float64
		if res.RVC != nil {
			ind, dir = res.RVC.Indirect, res.RVC.Direct
		}
		if !inRange(ind, e.IndirectMin, e.IndirectMax) || !inRange(dir, e.DirectMin, e.DirectMax) {
			if why, ok := knownDeviation[c.ID]; ok && ind != nil && dir != nil && pct(*ind) == "36.7" && pct(*dir) == "36.7" {
				t.Logf("%s: known deviation: %s", c.ID, why)
			} else {
				t.Errorf("%s: rvc %+v", c.ID, res.RVC)
			}
		}
		t.Logf("%s: %s %s rule=%s", c.ID, res.Outcome, res.Applied, res.Rule.Concise)
	}
}
