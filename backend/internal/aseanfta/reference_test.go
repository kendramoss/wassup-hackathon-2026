package aseanfta

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

type expectation struct {
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
}

// mismatches lists every pre-registered check the result fails.
func mismatches(res atiga.Result, e expectation) []string {
	var bad []string
	inRange := func(p, lo, hi *float64) bool {
		if lo == nil {
			return true
		}
		return p != nil && *p >= *lo && *p <= *hi
	}
	if e.Outcome != "" && res.Outcome != e.Outcome {
		bad = append(bad, "outcome "+res.Outcome+", want "+e.Outcome)
	}
	if e.Applied != "" && res.Applied != e.Applied {
		bad = append(bad, "applied "+res.Applied+", want "+e.Applied)
	}
	for rule, want := range e.Alternative {
		got := ""
		for _, a := range res.Alternatives {
			if a.Rule == rule {
				got = a.Outcome
			}
		}
		if got != want {
			bad = append(bad, "alternative "+rule+" = "+got+", want "+want)
		}
	}
	if e.Accumulation != nil && res.Accumulation != *e.Accumulation {
		bad = append(bad, "accumulation")
	}
	var ind, dir, ws, vs *float64
	if res.RVC != nil {
		ind, dir = res.RVC.Indirect, res.RVC.Direct
	}
	if res.DeMinimis != nil {
		ws, vs = res.DeMinimis.WeightShare, res.DeMinimis.ValueShare
	}
	if !inRange(ind, e.IndirectMin, e.IndirectMax) {
		bad = append(bad, "rvc indirect")
	}
	if !inRange(dir, e.DirectMin, e.DirectMax) {
		bad = append(bad, "rvc direct")
	}
	if e.DeMinimisUsed != nil && (res.DeMinimis != nil && res.DeMinimis.Used) != *e.DeMinimisUsed {
		bad = append(bad, "de minimis used")
	}
	if !inRange(ws, e.WeightShareMin, e.WeightShareMax) {
		bad = append(bad, "weight share")
	}
	if !inRange(vs, e.ValueShareMin, e.ValueShareMax) {
		bad = append(bad, "value share")
	}
	return bad
}

// firstRun records what the pre-registered inputs did on the first run
// (D950). Every other pre-registered case must match in full.
var firstRun = map[string]string{
	// The input named method direct; ACFTA has only the build-down formula.
	"ac-accumulation-with":    "refused",
	"ac-accumulation-without": "refused",
	// The bill listed only originating materials, so the check reported
	// RVC 100 %: the outcome and the alternative match, the direct figure
	// (the guide's 52 %) was not produced.
	"ak-wm-build-up": "rvc direct",
}

// TestReferenceCases runs the authority worked examples pre-registered in
// D949 (eval/akfta and eval/acfta reference-cases.json), and the post-hoc
// corrections of D950, and checks each stated result. They are teaching
// examples, not rulings, so the result is a count of agreements, not an
// accuracy rate.
func TestReferenceCases(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "eval")
	read := func(path string, v any) {
		b, err := os.ReadFile(path)
		if err == nil {
			err = json.Unmarshal(b, v)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, run := range []struct {
		ag   Agreement
		dir  string
		want int
	}{{AKFTA, "akfta", 8}, {ACFTA, "acfta", 5}} {
		var doc struct {
			Cases []struct {
				ID     string      `json:"id"`
				Input  atiga.Input `json:"input"`
				Expect expectation `json:"expect"`
			} `json:"cases"`
		}
		var post struct {
			Cases []struct {
				Reference string       `json:"reference"`
				Input     atiga.Input  `json:"input"`
				Expect    *expectation `json:"expect"`
			} `json:"cases"`
		}
		read(filepath.Join(root, run.dir, "reference-cases.json"), &doc)
		read(filepath.Join(root, run.dir, "post-hoc-cases.json"), &post)
		if len(doc.Cases) != run.want {
			t.Fatalf("%s: %d cases, want the %d pre-registered", run.dir, len(doc.Cases), run.want)
		}
		type postCase struct {
			in atiga.Input
			e  *expectation
		}
		postInput := map[string]postCase{}
		for _, p := range post.Cases {
			postInput[p.Reference] = postCase{p.Input, p.Expect}
		}
		for _, c := range doc.Cases {
			res, err := Check(run.ag, c.Input)
			bad := mismatches(res, c.Expect)
			switch want := firstRun[c.ID]; {
			case want == "refused":
				if !errors.Is(err, atiga.ErrInput) {
					t.Errorf("%s: pre-registered input should be refused, got %v", c.ID, err)
				}
			case err != nil:
				t.Errorf("%s: %v", c.ID, err)
			case want != "":
				if len(bad) != 1 || bad[0] != want {
					t.Errorf("%s: first-run deviation %v, want only %q", c.ID, bad, want)
				}
			case len(bad) > 0:
				t.Errorf("%s: %v; %v", c.ID, bad, res.Reasons)
			}
			if err == nil {
				t.Logf("%s/%s: first run %s %s, mismatches %v", run.dir, c.ID, res.Outcome, res.Applied, bad)
			}
			if pc, ok := postInput[c.ID]; ok {
				res, err := Check(run.ag, pc.in)
				if err != nil {
					t.Errorf("%s (post hoc): %v", c.ID, err)
					continue
				}
				e := c.Expect
				if pc.e != nil {
					e = *pc.e
				}
				if bad := mismatches(res, e); len(bad) > 0 {
					t.Errorf("%s (post hoc): %v; %v", c.ID, bad, res.Reasons)
				}
				t.Logf("%s/%s: post hoc %s %s rvc %+v", run.dir, c.ID, res.Outcome, res.Applied, res.RVC)
			}
		}
	}
}
