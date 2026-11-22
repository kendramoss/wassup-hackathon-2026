package main

// The D600 single-setup runs, scored offline (no model calls):
//
//	go run ./cmd/tariffshift p102ck-eval [-check]   (in make reproduce)
//
// The checklist variants of the Part 102 engine (usmca-v3, Vb, D590;
// usmca-v4, Vc, and usmca-v5, Vd, D600) on the 72 development Part 102
// rulings, against D550's usmca-v1 re-run (B, 53 of 72, $0.6881), and, only
// for the variant selected on development under D601, on the two held-out
// windows (final 32, final2 13 with the router) against the committed
// usmca-v1 runs. A file
// that is not there is reported as not run. A ruling whose engine call
// failed counts as wrong.

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
)

const (
	p102ckDir = evalDir + "/p102ck"
	p102ckOut = p102ckDir + "/summary-d602.json"
)

// p102ckRun is one scored run; Ref is the key of the run it is compared
// with ("" for a reference).
type p102ckRun struct {
	Key, Name, File, Ref string
	HeldOut              bool
}

var p102ckRuns = []p102ckRun{
	{"dev_b_v1_d550", "B: usmca-v1, D550 re-run (reference)", evalDir + "/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm-d550.jsonl", "", false},
	{"dev_vb_v3", "Vb: usmca-v3 checklist (D590), run under D601", p102ckDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v3-litellm-d601.jsonl", "dev_b_v1_d550", false},
	{"dev_vc_v4", "Vc: usmca-v4 compact checklist", p102ckDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v4-litellm-d600.jsonl", "dev_b_v1_d550", false},
	{"dev_vd_v5", "Vd: usmca-v5 failing-materials checklist", p102ckDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v5-litellm-d600.jsonl", "dev_b_v1_d550", false},
	{"final_v1", "usmca-v1, committed window 1 run (October)", "../eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl", "", true},
	{"final2_v1", "usmca-v1, committed window 2 run (November, router)", "../eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl", "", true},
}

// p102ckHeldOut are the held-out files of each candidate (D601: only the
// one selected on development is run, once).
var p102ckHeldOut = []struct{ Key, Name, Version string }{
	{"vb_v3", "Vb: usmca-v3", "usmca-v3"},
	{"vc_v4", "Vc: usmca-v4", "usmca-v4"},
	{"vd_v5", "Vd: usmca-v5", "usmca-v5"},
}

func init() {
	for _, h := range p102ckHeldOut {
		p102ckRuns = append(p102ckRuns,
			p102ckRun{"final_" + h.Key, h.Name + ", window 1", p102ckDir + "/runs/usmca-final-gemini-4-argon-prec-" + h.Version + "-litellm-d601.jsonl", "final_v1", true},
			p102ckRun{"final2_" + h.Key, h.Name + ", window 2 (router)", p102ckDir + "/runs/usmca-final2-gemini-4-argon-prec-" + h.Version + "-litellm-router-d601.jsonl", "final2_v1", true})
	}
}

// P102ckRun is one run's score.
type P102ckRun struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	File       string   `json:"file"`
	HeldOut    bool     `json:"held_out"`
	Run        bool     `json:"run"`
	N          int      `json:"n,omitempty"`
	Correct    int      `json:"correct,omitempty"`
	Errors     int      `json:"errors_left,omitempty"`
	GatewayUSD float64  `json:"gateway_cost_usd,omitempty"`
	PromoUSD   float64  `json:"promo_cost_usd,omitempty"`
	Ref        string   `json:"compared_with,omitempty"`
	Won        []string `json:"won,omitempty"`
	Lost       []string `json:"lost,omitempty"`
	McNemarP   *float64 `json:"mcnemar_p,omitempty"`
	CostRatio  *float64 `json:"gateway_cost_ratio,omitempty"`
	// Per-call latency of the answering call, median and 95th percentile
	// (reported, not a pass condition; D601).
	LatencyP50MS int64 `json:"latency_p50_ms,omitempty"`
	LatencyP95MS int64 `json:"latency_p95_ms,omitempty"`
	OutTokens    int   `json:"output_tokens,omitempty"`
}

// P102ckSummary is summary-d601.json.
type P102ckSummary struct {
	Note string      `json:"note"`
	Runs []P102ckRun `json:"runs"`
	// HeldOutPooled is each variant's correct count over both windows
	// (45), beside usmca-v1's 36, when both windows were run.
	HeldOutPooled map[string]int `json:"held_out_pooled_correct,omitempty"`
}

func cmdP102ckEval(args []string) error {
	fl := flag.NewFlagSet("p102ck-eval", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed summary differs")
	_ = fl.Parse(args)
	sum := P102ckSummary{Note: "D600/D601: the Part 102 engine with a compact 102.11 checklist (usmca-v4, Vc) or the failing materials only (usmca-v5, Vd) on the 72 development Part 102 rulings against D550's usmca-v1 re-run, and on the held-out windows only for a variant that passed on development, through an OpenAI-compatible LiteLLM gateway to Gemini. Recompute: cd backend && go run ./cmd/tariffshift p102ck-eval -check"}
	rows := map[string]map[string]g102Row{}
	scored := map[string]P102ckRun{}
	for _, r := range p102ckRuns {
		out := P102ckRun{Key: r.Key, Name: r.Name, File: trimDots(r.File), HeldOut: r.HeldOut, Ref: r.Ref}
		rs, err := readG102(r.File)
		if os.IsNotExist(err) {
			sum.Runs = append(sum.Runs, out)
			continue
		}
		if err != nil {
			return err
		}
		out.Run = true
		by := map[string]g102Row{}
		var gw, promo float64
		// A resumed run appends the re-run row: the last row per ruling
		// counts, and the cost is every row's (what was spent).
		for _, x := range rs {
			by[x.Ruling] = x
			gw += x.Gateway
			promo += x.Promo
		}
		var lat []int64
		for _, x := range by {
			if x.LatencyMS > 0 {
				lat = append(lat, x.LatencyMS)
			}
			out.OutTokens += x.OutTok
			out.N++
			if x.Error != "" {
				out.Errors++
			} else if x.Correct {
				out.Correct++
			}
		}
		out.GatewayUSD, out.PromoUSD = round4(gw), round4(promo)
		if len(lat) > 0 {
			sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
			out.LatencyP50MS, out.LatencyP95MS = pctl(lat, 0.50), pctl(lat, 0.95)
		}
		rows[r.Key] = by
		if ref, ok := rows[r.Ref]; ok {
			var refUSD float64
			for id, x := range by {
				b, ok := ref[id]
				if !ok {
					return fmt.Errorf("%s: %s is not in %s", r.Key, id, r.Ref)
				}
				refUSD += b.Gateway
				xc, bc := x.Error == "" && x.Correct, b.Error == "" && b.Correct
				switch {
				case xc && !bc:
					out.Won = append(out.Won, id)
				case !xc && bc:
					out.Lost = append(out.Lost, id)
				}
			}
			if len(by) != len(ref) {
				return fmt.Errorf("%s: %d rulings against %d in %s", r.Key, len(by), len(ref), r.Ref)
			}
			sort.Strings(out.Won)
			sort.Strings(out.Lost)
			p := round4(mcnemarExact(len(out.Won), len(out.Lost)))
			out.McNemarP = &p
			if refUSD > 0 && !r.HeldOut {
				ratio := round4(gw / refUSD)
				out.CostRatio = &ratio
			}
		}
		scored[r.Key] = out
		sum.Runs = append(sum.Runs, out)
	}
	pooled := []struct{ key, w1, w2 string }{{"usmca-v1", "final_v1", "final2_v1"}}
	for _, h := range p102ckHeldOut {
		pooled = append(pooled, struct{ key, w1, w2 string }{h.Key, "final_" + h.Key, "final2_" + h.Key})
	}
	for _, v := range pooled {
		a, ok1 := scored[v.w1]
		b, ok2 := scored[v.w2]
		if ok1 && ok2 {
			if sum.HeldOutPooled == nil {
				sum.HeldOutPooled = map[string]int{}
			}
			sum.HeldOutPooled[v.key] = a.Correct + b.Correct
		}
	}
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if err := writeOrCheckPlans(p102ckOut, append(sb, '\n'), *check); err != nil {
		return err
	}
	for _, r := range sum.Runs {
		if !r.Run {
			fmt.Printf("p102ck-eval %s: not run\n", r.Key)
			continue
		}
		fmt.Printf("p102ck-eval %s: %d/%d, %d errors, $%.4f\n", r.Key, r.Correct, r.N, r.Errors, r.GatewayUSD)
	}
	return nil
}

// trimDots is path without the leading "../" the backend directory adds.
func trimDots(path string) string {
	for len(path) > 3 && path[:3] == "../" {
		path = path[3:]
	}
	return path
}

// pctl is the nearest-rank percentile of sorted xs.
func pctl(xs []int64, q float64) int64 {
	i := int(math.Ceil(q*float64(len(xs)))) - 1
	return xs[max(0, min(i, len(xs)-1))]
}
