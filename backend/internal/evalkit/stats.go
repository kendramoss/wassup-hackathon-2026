package evalkit

import (
	"math"
	"sort"
	"strings"
)

// Wilson returns the proportion k/n and its Wilson 95% interval.
func Wilson(k, n int) (p, lo, hi float64) {
	if n == 0 {
		return 0, 0, 0
	}
	const z = 1.96
	nf := float64(n)
	p = float64(k) / nf
	d := 1 + z*z/nf
	c := (p + z*z/(2*nf)) / d
	h := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / d
	return p, c - h, c + h
}

// Percentile returns the q-th percentile (0–100) by nearest rank.
func Percentile(values []int64, q float64) int64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]int64(nil), values...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	rank := int(math.Ceil(q / 100 * float64(len(v))))
	if rank < 1 {
		rank = 1
	}
	return v[rank-1]
}

// Price is USD per 1M input and output tokens (output includes thinking).
type Price struct{ In, Out float64 }

// Prices are Gemini Developer API paid-tier prices verified 2026-10-14
// (hackathon-research/evidence/pricing-2026-10-14.md). Vertex AI prices in
// the team's project are not yet checked. Promo prices end 2026-11-30 (Argon)
// and 2026-12-31 (Flash); List is the price after that.
var (
	PromoPrices = map[string]Price{"gemini-4-argon": {2.0, 10.0}, "gemini-3.8-flash": {0.75, 3.75}}
	ListPrices  = map[string]Price{"gemini-4-argon": {4.0, 20.0}, "gemini-3.8-flash": {1.5, 7.5}}
)

// Cost returns the USD cost of one call, or 0 for an unpriced model.
func Cost(prices map[string]Price, model string, in, out int) float64 {
	for name, p := range prices {
		if strings.HasPrefix(model, name) {
			return (float64(in)*p.In + float64(out)*p.Out) / 1e6
		}
	}
	return 0
}

// Bin is accuracy among rows with confidence in [Min, Max).
type Bin struct {
	Label    string
	Min, Max float64
	N, K     int
}

// Summary is everything the eval reports for one run.
type Summary struct {
	N, Correct, Errors int
	Acc, Lo, Hi        float64
	Bins               []Bin
	// AtLeast95 / AtLeast90 are cumulative calibration rows (confidence ≥ x).
	AtLeast95, AtLeast90 Bin
	FastN, FastK         int // triage "auto" rows
	Providers, Models    []string
	PromptVersions       []string
	CostPerCase          float64
	CostPerCaseList      float64
	LatencyP50, P95      int64
	MeanInTok, MeanOut   float64
	ByMethod             map[string][2]int // label method -> {n, correct}
}

// Summarize computes a Summary. Rows with an error count as wrong (the
// prediction failed) and are also counted in Errors.
func Summarize(rows []Row) Summary {
	s := Summary{
		Bins: []Bin{
			{Label: "≥ 0.95", Min: 0.95, Max: 2}, {Label: "0.90–0.95", Min: 0.90, Max: 0.95},
			{Label: "0.80–0.90", Min: 0.80, Max: 0.90}, {Label: "< 0.80", Min: -1, Max: 0.80},
		},
		AtLeast95: Bin{Label: "≥ 0.95", Min: 0.95, Max: 2},
		AtLeast90: Bin{Label: "≥ 0.90", Min: 0.90, Max: 2},
		ByMethod:  map[string][2]int{},
	}
	providers, models, prompts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var lat []int64
	var cost, costList, in, out float64
	for _, r := range rows {
		s.N++
		if r.Error != "" {
			s.Errors++
		}
		k := 0
		if r.Correct && r.Error == "" {
			s.Correct++
			k = 1
		}
		m := s.ByMethod[r.LabelMethod]
		s.ByMethod[r.LabelMethod] = [2]int{m[0] + 1, m[1] + k}
		if r.Error != "" {
			continue
		}
		for i := range s.Bins {
			if r.Confidence >= s.Bins[i].Min && r.Confidence < s.Bins[i].Max {
				s.Bins[i].N++
				s.Bins[i].K += k
			}
		}
		for _, b := range []*Bin{&s.AtLeast95, &s.AtLeast90} {
			if r.Confidence >= b.Min {
				b.N++
				b.K += k
			}
		}
		if r.TriageRoute == "auto" {
			s.FastN++
			s.FastK += k
		}
		providers[r.Provider], models[r.ModelID], prompts[r.PromptVersion] = true, true, true
		lat = append(lat, r.LatencyMS)
		cost += r.CostUSD
		costList += r.CostUSDList
		in += float64(r.InputTokens)
		out += float64(r.OutputTokens)
	}
	s.Acc, s.Lo, s.Hi = Wilson(s.Correct, s.N)
	if ok := s.N - s.Errors; ok > 0 {
		s.CostPerCase, s.CostPerCaseList = cost/float64(ok), costList/float64(ok)
		s.MeanInTok, s.MeanOut = in/float64(ok), out/float64(ok)
	}
	s.LatencyP50, s.P95 = Percentile(lat, 50), Percentile(lat, 95)
	s.Providers, s.Models, s.PromptVersions = keys(providers), keys(models), keys(prompts)
	return s
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
