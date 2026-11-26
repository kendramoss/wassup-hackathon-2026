// Command usmcaeval scores the USMCA origin-rule check against CBP rulings
// that decide USMCA eligibility under HTSUS General Note 11 (D-entry
// "USMCA rule check beyond the pilot").
//
//	go run ./cmd/usmcaeval -split dev            # writes results and summary
//	go run ./cmd/usmcaeval -split dev -check     # recomputes, fails on drift
//
// Cases come from eval/usmca/cases-<split>.jsonl (extract_cases.py). The
// check is deterministic and makes no model call. Each scored product is run
// through the pilot rule set and the extended one (GN 11(o) rows, de minimis);
// met counts as "qualifies", not_met as "does not qualify", anything else is
// undecided.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/usmca"
)

type material struct {
	Name              string `json:"name"`
	HS                string `json:"hs"`
	Country           string `json:"country"`
	StatedOriginating string `json:"stated_originating"`
	Value             string `json:"value"`
}

type product struct {
	Name                  string     `json:"name"`
	GoodHS                string     `json:"good_hs"`
	TransactionValue      string     `json:"transaction_value"`
	NetCost               string     `json:"net_cost"`
	VNM                   string     `json:"vnm"`
	ProducedIn            string     `json:"produced_in"`
	Question              string     `json:"question"`
	CBPAnswer             string     `json:"cbp_answer"`
	CBPBasis              string     `json:"cbp_basis"`
	Materials             []material `json:"materials"`
	MaterialsListComplete bool       `json:"materials_list_complete"`
}

type caseRow struct {
	Ruling   string    `json:"ruling"`
	Date     string    `json:"date"`
	Products []product `json:"products"`
}

type result struct {
	Ruling    string `json:"ruling"`
	Product   int    `json:"product"`
	GoodHS    string `json:"good_hs"`
	CBP       string `json:"cbp"`
	Pilot     string `json:"pilot"`
	Extended  string `json:"extended"`
	Row       string `json:"row,omitempty"`
	Why       string `json:"why,omitempty"`
	Skip      string `json:"skip,omitempty"`
	DeMinimis bool   `json:"de_minimis,omitempty"`
}

var nonDigit = regexp.MustCompile(`[^0-9]`)

var usmcaCountries = map[string]bool{"united states": true, "us": true, "u.s.": true, "usa": true, "u.s.a.": true,
	"mexico": true, "canada": true, "the united states": true}

func digits(s string) string { return nonDigit.ReplaceAllString(s, "") }

func amount(s string) usmca.Amount {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	s = strings.TrimPrefix(s, "$")
	a, err := usmca.ParseAmount(s)
	if err != nil {
		return usmca.Unknown()
	}
	return a
}

// claim is the pre-registered origin rule: the ruling's own statement first,
// then a country inside the USMCA territories counts as originating and any
// other stated country as non-originating; no country is unknown.
func claim(m material) usmca.OriginClaim {
	switch strings.ToLower(m.StatedOriginating) {
	case "yes":
		return usmca.ClaimOriginating
	case "no":
		return usmca.ClaimNonOriginating
	}
	c := strings.ToLower(strings.TrimSpace(m.Country))
	if c == "" {
		return usmca.ClaimUnknown
	}
	if usmcaCountries[c] {
		return usmca.ClaimOriginating
	}
	return usmca.ClaimNonOriginating
}

func input(p product) (usmca.Input, string) {
	good := digits(p.GoodHS)
	switch len(good) {
	case 4, 6, 8, 10:
	default:
		return usmca.Input{}, "no usable good code"
	}
	in := usmca.Input{Destination: usmca.DestinationUS, Regime: usmca.RegimeUSMCA, GoodHS: good,
		TransactionValue: amount(p.TransactionValue), NetCost: amount(p.NetCost), VNM: amount(p.VNM)}
	anyValue := false
	for _, m := range p.Materials {
		hs := digits(m.HS)
		if len(hs)%2 == 1 {
			hs = hs[:len(hs)-1]
		}
		if len(hs) > 10 {
			hs = hs[:10]
		}
		v := amount(m.Value)
		anyValue = anyValue || v.IsKnown()
		in.Materials = append(in.Materials, usmca.Material{ID: m.Name[:min(len(m.Name), usmca.MaxMaterialID)], HS: hs, Originating: claim(m), Value: v})
	}
	if len(in.Materials) > usmca.MaxMaterials {
		return usmca.Input{}, "too many materials"
	}
	switch {
	case in.TransactionValue.IsKnown() && in.VNM.IsKnown():
		in.RVCMethod = string(usmca.MethodTransactionValue)
	case in.NetCost.IsKnown() && in.VNM.IsKnown():
		in.RVCMethod = string(usmca.MethodNetCost)
	}
	na := usmca.ProvisionNotApplicable
	// An empty materials list is never read as "no non-originating materials".
	in.Profile = usmca.Profile{SimpleGoods: true, MaterialsComplete: len(in.Materials) > 0, Sets: na, AccessoriesPacking: na, Fungibles: na,
		IntermediateMaterials: na, CostAveraging: na, DeMinimis: na, OutsidePartyOperations: na, SpecialValuation: na, PartsRoute: na}
	pi := strings.ToLower(p.ProducedIn)
	in.Profile.ImportedFromUSMCA = strings.Contains(pi, "mexico") || strings.Contains(pi, "canada")
	if anyValue && in.TransactionValue.IsKnown() {
		in.Profile.DeMinimis = usmca.ProvisionApplies
		in.DeMinimisBasis = usmca.DeMinimisBasisTransactionValue
	}
	return in, ""
}

func answer(rs *usmca.RuleSet, in usmca.Input) (string, string, bool, string) {
	out, err := rs.Check(in)
	if err != nil {
		return "error", "", false, err.Error()
	}
	var why []string
	seen := map[string]bool{}
	for _, f := range out.MissingFacts {
		if !seen[f.Code] {
			seen[f.Code] = true
			why = append(why, f.Code)
		}
	}
	if len(out.UnsupportedReasons) > 0 {
		why = append(why, out.UnsupportedReasons[0])
	}
	sort.Strings(why)
	dm := false
	for _, a := range out.Alternatives {
		if strings.Contains(a.Note, "de minimis") {
			dm = true
		}
	}
	switch out.Status {
	case usmca.RuleMet:
		return "qualifies", out.RowID, dm, ""
	case usmca.RuleNotMet:
		return "does_not_qualify", out.RowID, dm, ""
	}
	return "undecided:" + string(out.Status), out.RowID, dm, strings.Join(why, ",")
}

type tally struct {
	Scored  int     `json:"scored"`
	Decided int     `json:"decided"`
	Agreed  int     `json:"agreed"`
	Agree   float64 `json:"agreement_on_decided"`
	Lo      float64 `json:"agreement_wilson_lo"`
	Hi      float64 `json:"agreement_wilson_hi"`
	// confusion: cbp answer -> engine answer
	Confusion map[string]map[string]int `json:"confusion"`
}

func wilson(k, n int) (float64, float64) {
	if n == 0 {
		return 0, 0
	}
	z, p, nf := 1.959964, float64(k)/float64(n), float64(n)
	d := 1 + z*z/nf
	c := (p + z*z/(2*nf)) / d
	h := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / d
	return math.Round((c-h)*1e4) / 1e4, math.Round((c+h)*1e4) / 1e4
}

func score(rs []result, pick func(result) string) tally {
	t := tally{Confusion: map[string]map[string]int{}}
	for _, r := range rs {
		if r.Skip != "" {
			continue
		}
		t.Scored++
		got := pick(r)
		short := got
		if strings.HasPrefix(got, "undecided") {
			short = "undecided"
		}
		if t.Confusion[r.CBP] == nil {
			t.Confusion[r.CBP] = map[string]int{}
		}
		t.Confusion[r.CBP][short]++
		if short == "qualifies" || short == "does_not_qualify" {
			t.Decided++
			if short == r.CBP {
				t.Agreed++
			}
		}
	}
	if t.Decided > 0 {
		t.Agree = math.Round(float64(t.Agreed)/float64(t.Decided)*1e4) / 1e4
	}
	t.Lo, t.Hi = wilson(t.Agreed, t.Decided)
	return t
}

func run(repo, split string) ([]result, map[string]any, error) {
	f, err := os.Open(filepath.Join(repo, "eval/usmca", "cases-"+split+".jsonl"))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	pilot, err := usmca.DefaultRules()
	if err != nil {
		return nil, nil, err
	}
	ext, err := usmca.ExtendedRules()
	if err != nil {
		return nil, nil, err
	}
	var rs []result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	rulings := 0
	for sc.Scan() {
		var c caseRow
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, nil, err
		}
		rulings++
		for i, p := range c.Products {
			r := result{Ruling: c.Ruling, Product: i, GoodHS: digits(p.GoodHS), CBP: p.CBPAnswer}
			switch {
			case p.Question != "usmca_eligibility":
				r.Skip = "not a USMCA eligibility question"
			case p.CBPAnswer != "qualifies" && p.CBPAnswer != "does_not_qualify":
				r.Skip = "CBP answer unclear"
			}
			if r.Skip == "" {
				in, why := input(p)
				if why != "" {
					r.Skip = why
				} else {
					r.Pilot, _, _, _ = answer(pilot, in)
					r.Extended, r.Row, r.DeMinimis, r.Why = answer(ext, in)
				}
			}
			rs = append(rs, r)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	skips := map[string]int{}
	for _, r := range rs {
		if r.Skip != "" {
			skips[r.Skip]++
		}
	}
	ext2 := score(rs, func(r result) string { return r.Extended })
	sum := map[string]any{
		"split": split, "rulings": rulings, "products": len(rs), "skipped": skips,
		"pilot":    score(rs, func(r result) string { return r.Pilot }),
		"extended": ext2,
		"extended_by_answer": map[string]any{
			"qualifies_decided_agreed":        fmt.Sprintf("%d/%d", ext2.Confusion["qualifies"]["qualifies"], ext2.Confusion["qualifies"]["qualifies"]+ext2.Confusion["qualifies"]["does_not_qualify"]),
			"does_not_qualify_decided_agreed": fmt.Sprintf("%d/%d", ext2.Confusion["does_not_qualify"]["does_not_qualify"], ext2.Confusion["does_not_qualify"]["qualifies"]+ext2.Confusion["does_not_qualify"]["does_not_qualify"]),
		},
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Ruling != rs[j].Ruling {
			return rs[i].Ruling < rs[j].Ruling
		}
		return rs[i].Product < rs[j].Product
	})
	return rs, sum, nil
}

func main() {
	repo := flag.String("repo", "..", "repository root")
	split := flag.String("split", "dev", "dev or held_out")
	check := flag.Bool("check", false, "recompute and fail if the committed results differ")
	flag.Parse()
	if *split != "dev" && *split != "held_out" {
		fail(errors.New("-split must be dev or held_out"))
	}
	rs, sum, err := run(*repo, *split)
	if err != nil {
		fail(err)
	}
	var lines strings.Builder
	for _, r := range rs {
		b, _ := json.Marshal(r)
		lines.Write(b)
		lines.WriteByte('\n')
	}
	sb, _ := json.MarshalIndent(sum, "", " ")
	sb = append(sb, '\n')
	rp := filepath.Join(*repo, "eval/usmca", "results-"+*split+".jsonl")
	sp := filepath.Join(*repo, "eval/usmca", "summary-"+*split+".json")
	if *check {
		for p, want := range map[string]string{rp: lines.String(), sp: string(sb)} {
			got, err := os.ReadFile(p)
			if err != nil || string(got) != want {
				fail(fmt.Errorf("%s is stale; rerun go run ./cmd/usmcaeval -split %s", p, *split))
			}
		}
		fmt.Println("usmcaeval ok", *split)
		return
	}
	if err := os.WriteFile(rp, []byte(lines.String()), 0o644); err != nil {
		fail(err)
	}
	if err := os.WriteFile(sp, sb, 0o644); err != nil {
		fail(err)
	}
	fmt.Println(string(sb))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "usmcaeval:", err)
	os.Exit(1)
}
