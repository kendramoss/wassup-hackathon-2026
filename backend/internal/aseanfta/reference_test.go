package aseanfta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

// TestReferenceCases runs the authority worked examples pre-registered in
// D949 (eval/akfta and eval/acfta reference-cases.json) and checks each
// stated result. They are teaching examples, not rulings, so the result is
// a count of agreements, not an accuracy rate.
func TestReferenceCases(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "eval")
	for _, run := range []struct {
		ag   Agreement
		dir  string
		want int
	}{{AKFTA, "akfta", 8}, {ACFTA, "acfta", 5}} {
		b, err := os.ReadFile(filepath.Join(root, run.dir, "reference-cases.json"))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Cases []struct {
				ID     string      `json:"id"`
				Input  atiga.Input `json:"input"`
				Expect struct {
					Outcome        string            `json:"outcome"`
					Applied        string            `json:"applied"`
					Alternative    map[string]string `json:"alternative"`
					Accumulation   *bool             `json:"accumulation"`
					IndirectMin    *float64          `json:"rvc_indirect_min"`
					IndirectMax    *float64          `json:"rvc_indirect_max"`
					DirectMin      *float64          `json:"rvc_direct_min"`
					DirectMax      *float64          `json:"rvc_direct_max"`
					DeMinimisUsed  *bool             `json:"de_minimis_used"`
					WeightShareMin *float64          `json:"weight_share_min"`
					WeightShareMax *float64          `json:"weight_share_max"`
					ValueShareMin  *float64          `json:"value_share_min"`
					ValueShareMax  *float64          `json:"value_share_max"`
				} `json:"expect"`
			} `json:"cases"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Cases) != run.want {
			t.Fatalf("%s: %d cases, want the %d pre-registered", run.dir, len(doc.Cases), run.want)
		}
		inRange := func(p, lo, hi *float64) bool {
			if lo == nil {
				return true
			}
			return p != nil && *p >= *lo && *p <= *hi
		}
		for _, c := range doc.Cases {
			res, err := Check(run.ag, c.Input)
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
				t.Errorf("%s: rvc %+v", c.ID, res.RVC)
			}
			if e.DeMinimisUsed != nil && (res.DeMinimis != nil && res.DeMinimis.Used) != *e.DeMinimisUsed {
				t.Errorf("%s: de minimis %+v", c.ID, res.DeMinimis)
			}
			var ws, vs *float64
			if res.DeMinimis != nil {
				ws, vs = res.DeMinimis.WeightShare, res.DeMinimis.ValueShare
			}
			if !inRange(ws, e.WeightShareMin, e.WeightShareMax) || !inRange(vs, e.ValueShareMin, e.ValueShareMax) {
				t.Errorf("%s: de minimis shares %+v", c.ID, res.DeMinimis)
			}
			rule := ""
			if res.Rule != nil {
				rule = res.Rule.Concise
			}
			t.Logf("%s/%s: %s %s rule=%s", run.dir, c.ID, res.Outcome, res.Applied, rule)
		}
	}
}
