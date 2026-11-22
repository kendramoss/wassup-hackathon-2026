package main

// The D590 per-move runs, scored offline (no model calls):
//
//	go run ./cmd/tariffshift p102err-moves [-check]   (in make reproduce)
//
// Each series is one or more files written by `eval p102moves` (the Part
// 102 engine alone on the current setup and on each move of the
// development plan fixtures that the 19 CFR 102.20 check decides or leans
// on). A row agrees with the rule when, for a move, the engine predicts the
// target exactly when the rule gives the target, and for a current setup
// when the engine names the rule's country. A lean is the rule's answer when
// only the de minimis value is open. A row whose engine call failed counts
// as a disagreement.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
)

// p102errSeries are the scored series, in order; b is the reference.
var p102errSeries = []struct {
	Key, Name string
	Files     []string
}{
	{"b", "B: usmca-v1, facts as plan searches render them (production)", []string{"moves-d590-b-d501fam.jsonl", "moves-d590-b-dev.jsonl"}},
	{"va", "Va: usmca-v1, the product's codes added to the facts", []string{"moves-d590-va.jsonl"}},
	{"vb", "Vb: usmca-v3 checklist, the product's codes added to the facts", []string{"moves-d590-vb.jsonl"}},
}

type p102Row struct {
	Set     string  `json:"set"`
	Fixture string  `json:"fixture"`
	Target  string  `json:"target"`
	CBP     string  `json:"cbp_origin"`
	Kind    string  `json:"kind"`
	Move    string  `json:"move"`
	Status  string  `json:"status"`
	Origin  string  `json:"origin"`
	Lean    string  `json:"if_not_de_minimis"`
	Engine  string  `json:"engine_origin"`
	Error   string  `json:"error"`
	USD     float64 `json:"gateway_cost_usd"`
	OutTok  int     `json:"output_tokens"`
	InTok   int     `json:"input_tokens"`
	Prompt  string  `json:"prompt_version"`
	Reason  string  `json:"essential_character"`
	Reasons string  `json:"reasoning"`
}

func (r p102Row) key() string { return r.Set + "/" + r.Fixture + "/" + r.Kind + "/" + r.Move }

// agrees reports whether the engine agrees with the rule (decided) or its
// lean; ok is false when the rule gives neither.
func (r p102Row) agrees() (agree, decided, ok bool) {
	if r.Error != "" || r.Engine == "" {
		return false, r.Status != planrule.Undetermined, r.Status != planrule.Undetermined || r.Lean != ""
	}
	if r.Kind == "baseline" {
		switch {
		case r.Status != planrule.Undetermined:
			return r.Engine == r.Origin, true, true
		case r.Lean != "":
			return r.Engine == r.Lean, false, true
		}
		return false, false, false
	}
	predictsTarget := r.Engine == r.Target
	switch {
	case r.Status != planrule.Undetermined:
		return predictsTarget == (r.Status == planrule.Shifts), true, true
	case r.Lean != "":
		return predictsTarget == (r.Lean == r.Target), false, true
	}
	return false, false, false
}

// P102Count is k of n with the rows that disagree.
type P102Count struct {
	K        int      `json:"k"`
	N        int      `json:"n"`
	Disagree []string `json:"disagree,omitempty"`
}

func (c *P102Count) add(ok bool, what string) {
	c.N++
	if ok {
		c.K++
	} else if what != "" {
		c.Disagree = append(c.Disagree, what)
	}
}

// P102Series is one scored series.
type P102Series struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Run   bool   `json:"run"`
	Calls int    `json:"calls"`
	// Errors are engine calls that failed after one re-run.
	Errors int `json:"errors"`
	// Decided: moves and current setups the rule decides, pooled (the D590
	// primary measure); DecidedMoves and DecidedSetups split it; Lean: rows
	// the rule leaves open only for de minimis.
	Decided       P102Count            `json:"decided"`
	DecidedMoves  P102Count            `json:"decided_moves"`
	DecidedSetups P102Count            `json:"decided_current_setups"`
	Lean          P102Count            `json:"lean"`
	BySet         map[string]P102Count `json:"decided_by_set"`
	// CBP: the engine on the current setups of the Part 102 fixtures
	// against CBP's holding.
	CBP        P102Count `json:"part102_current_setup_engine_agrees_with_cbp"`
	GatewayUSD float64   `json:"gateway_cost_usd"`
	OutTokens  int       `json:"output_tokens"`
	InTokens   int       `json:"input_tokens"`
	// Against B on the decided rows: B agrees and this series does not
	// (Lost), the reverse (Won), and the exact two-sided McNemar p.
	Won      []string `json:"won_vs_b,omitempty"`
	Lost     []string `json:"lost_vs_b,omitempty"`
	McNemarP *float64 `json:"mcnemar_p_vs_b,omitempty"`
	// CostRatio is this series' gateway cost over B's on the same calls.
	CostRatio *float64 `json:"cost_ratio_vs_b,omitempty"`
	// SafetyFlags: decided rows the D590 safety change S would flag (a
	// current setup the rule gives another country than the engine; a move
	// the rule says shifts while the engine predicts another country), of
	// the decided rows.
	SafetyFlags P102Count `json:"safety_flags_s"`
}

// P102MovesSummary is summary-moves.json.
type P102MovesSummary struct {
	Note   string       `json:"note"`
	Series []P102Series `json:"series"`
}

func cmdP102ErrMoves(args []string) error {
	fl := flag.NewFlagSet("p102err-moves", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed summary differs")
	_ = fl.Parse(args)
	sum := P102MovesSummary{Note: "D590: the Part 102 engine alone on the current setup and each move of the development plan fixtures (D501 three, the D590 development set, the D171 family fixtures with the top D381 suggestions) that the 19 CFR 102.20 check decides or leans on, one run per series through an OpenAI-compatible LiteLLM gateway to Gemini. Agreement with the rule is not accuracy: the rule reads unverified suggested part codes (D382). Recompute: cd backend && go run ./cmd/tariffshift p102err-moves -check"}
	var bRows map[string]p102Row
	var bUSD map[string]float64
	for _, s := range p102errSeries {
		out := P102Series{Key: s.Key, Name: s.Name, BySet: map[string]P102Count{}}
		var rows []p102Row
		missing := false
		for _, f := range s.Files {
			rs, err := readJSONL[p102Row](filepath.Join(p102errDir, "runs", f))
			if os.IsNotExist(err) {
				missing = true
				continue
			}
			if err != nil {
				return err
			}
			rows = append(rows, rs...)
		}
		if missing {
			sum.Series = append(sum.Series, out)
			continue
		}
		out.Run = true
		sort.Slice(rows, func(i, j int) bool { return rows[i].key() < rows[j].key() })
		byKey := map[string]p102Row{}
		usdBy := map[string]float64{}
		for _, r := range rows {
			byKey[r.key()] = r
			usdBy[r.key()] = r.USD
			out.Calls++
			out.GatewayUSD += r.USD
			out.InTokens += r.InTok
			out.OutTokens += r.OutTok
			if r.Error != "" {
				out.Errors++
			}
			what := fmt.Sprintf("%s: engine %s, rule %s", r.key(), orNone(r.Engine), ruleWords(r))
			if r.Kind == "baseline" && (r.Set == "part102-d501" || r.Set == "part102-dev") {
				out.CBP.add(r.Engine != "" && r.Engine == r.CBP, fmt.Sprintf("%s: engine %s, CBP %s", r.key(), orNone(r.Engine), r.CBP))
			}
			agree, decided, ok := r.agrees()
			if !ok {
				continue
			}
			if !decided {
				out.Lean.add(agree, what)
				continue
			}
			out.Decided.add(agree, what)
			flag := r.Error == "" && r.Engine != "" && ((r.Kind == "baseline" && r.Engine != r.Origin) ||
				(r.Kind == "move" && r.Status == planrule.Shifts && r.Engine != r.Target))
			out.SafetyFlags.N++
			if flag {
				out.SafetyFlags.K++
				out.SafetyFlags.Disagree = append(out.SafetyFlags.Disagree, r.key())
			}
			c := out.BySet[r.Set]
			c.add(agree, "")
			out.BySet[r.Set] = c
			if r.Kind == "baseline" {
				out.DecidedSetups.add(agree, "")
			} else {
				out.DecidedMoves.add(agree, "")
			}
		}
		out.GatewayUSD = round4(out.GatewayUSD)
		if s.Key == "b" {
			bRows, bUSD = byKey, usdBy
		} else if bRows != nil {
			won, lost := 0, 0
			var vUSD, refUSD float64
			for k, r := range byKey {
				br, ok := bRows[k]
				if !ok {
					return fmt.Errorf("%s: %s has no B row", s.Key, k)
				}
				vUSD += usdBy[k]
				refUSD += bUSD[k]
				a, d, ok := r.agrees()
				ba, _, _ := br.agrees()
				if !ok || !d {
					continue
				}
				switch {
				case a && !ba:
					won++
					out.Won = append(out.Won, k)
				case !a && ba:
					lost++
					out.Lost = append(out.Lost, k)
				}
			}
			sort.Strings(out.Won)
			sort.Strings(out.Lost)
			p := round4(mcnemarExact(won, lost))
			out.McNemarP = &p
			if refUSD > 0 {
				ratio := round4(vUSD / refUSD)
				out.CostRatio = &ratio
			}
		}
		sum.Series = append(sum.Series, out)
	}
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if err := writeOrCheckPlans(p102errDir+"/summary-moves.json", append(sb, '\n'), *check); err != nil {
		return err
	}
	for _, s := range sum.Series {
		if !s.Run {
			fmt.Printf("p102err-moves %s: not run\n", s.Key)
			continue
		}
		fmt.Printf("p102err-moves %s: decided %d/%d (moves %d/%d, setups %d/%d), lean %d/%d, CBP %d/%d, $%.4f, %d errors\n", s.Key,
			s.Decided.K, s.Decided.N, s.DecidedMoves.K, s.DecidedMoves.N, s.DecidedSetups.K, s.DecidedSetups.N, s.Lean.K, s.Lean.N, s.CBP.K, s.CBP.N, s.GatewayUSD, s.Errors)
	}
	return nil
}

func ruleWords(r p102Row) string {
	switch {
	case r.Status != planrule.Undetermined:
		return r.Status + " " + r.Origin
	case r.Lean != "":
		return "leans " + r.Lean
	}
	return "undetermined"
}
