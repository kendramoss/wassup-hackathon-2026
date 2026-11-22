package main

// The usmca-v5 auto-accept bar (D610) and the Part 102 latency tail (D611),
// from committed rows, no model calls:
//
//	go run ./cmd/tariffshift p102bar [-check]   (in make reproduce)
//
// D610 fits one calibration map (the cal-v2 procedure: blocks of at least 20
// cases, isotonic, capped at 0.99) on the 72 development rows of usmca-v5,
// picks its bar by the pre-registered target (at least 95% right on at
// least 20 auto-accepted development cases) and applies it to the committed
// held-out rows only if it auto-accepts more development cases than stated
// 0.95 does. D611 reads the latency of every committed Part 102 run and, when
// they are there, the two development re-run arms (without and with a
// hedged request).

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
)

const (
	p102barDir = evalDir + "/p102bar"
	p102barOut = p102barDir + "/summary.json"
	// D610's pre-registered numbers.
	p102barTarget   = 0.95 // right among the auto-accepted development cases
	p102barMinAuto  = 20   // auto-accepted development cases, at least
	p102barMinBlock = 20   // cases per block before the isotonic fit (cal-v2)
	p102barCap      = 0.99
	p102barStated   = 0.95 // the deployed bar on stated confidence (D61)
	// D611's hedge delay, in milliseconds.
	p102barHedgeMS = 12000
)

var (
	p102barDev   = p102ckDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v5-litellm-d600.jsonl"
	p102barDevV1 = evalDir + "/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm-d550.jsonl"
	// The committed held-out runs, window 1 then window 2.
	p102barHeld = map[string][2]string{
		"usmca-v1": {"../eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl", "../eval/results/usmca-final2-gemini-4-argon-prec-usmca-v1-litellm-router.jsonl"},
		"usmca-v5": {p102ckDir + "/runs/usmca-final-gemini-4-argon-prec-usmca-v5-litellm-d601.jsonl", p102ckDir + "/runs/usmca-final2-gemini-4-argon-prec-usmca-v5-litellm-router-d601.jsonl"},
	}
	p102barArmA = p102barDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v5-litellm-d611-a.jsonl"
	p102barArmB = p102barDir + "/runs/usmca-dev-gemini-4-argon-prec-usmca-v5-litellm-d611-b.jsonl"
)

// BarPoint is one bar on a set of rows: the cases at or above it.
type BarPoint struct {
	Bar      float64 `json:"bar"`
	N        int     `json:"n"`
	Auto     int     `json:"auto"`
	Right    int     `json:"right"`
	Accuracy float64 `json:"accuracy,omitempty"`
	CILow    float64 `json:"ci95_low_pct,omitempty"`
	CIHigh   float64 `json:"ci95_high_pct,omitempty"`
}

// MapStep is one block of the fitted map.
type MapStep struct {
	From       float64 `json:"from"`
	To         float64 `json:"to_incl"`
	N          int     `json:"n"`
	Correct    int     `json:"correct"`
	Calibrated float64 `json:"calibrated"`
}

// HeldBar is the held-out table for one engine at one bar.
type HeldBar struct {
	Engine  string   `json:"engine"`
	Bar     float64  `json:"bar"`
	Window1 BarPoint `json:"window_1"`
	Window2 BarPoint `json:"window_2"`
	Pooled  BarPoint `json:"pooled"`
}

// P102BarRule is D610's outcome.
type P102BarRule struct {
	Target     float64 `json:"target_accuracy"`
	MinAuto    int     `json:"min_auto_accepted"`
	DevFile    string  `json:"fitted_on"`
	DevN       int     `json:"dev_n"`
	DevCorrect int     `json:"dev_correct"`
	// StatedCurve is every stated value usmca-v5 emitted on development, as
	// a bar; StatedRuleBar the lowest one meeting the target (nil: none).
	StatedCurve   []BarPoint `json:"dev_stated_curve"`
	StatedRuleBar *float64   `json:"dev_stated_rule_bar"`
	// DevV1Curve is usmca-v1's development curve (D550 re-run), for reference.
	DevV1Curve []BarPoint `json:"dev_usmca_v1_stated_curve"`
	Map        []MapStep  `json:"map_cal_p102_v1"`
	MapCurve   []BarPoint `json:"dev_map_curve"`
	MapBar     *float64   `json:"dev_map_rule_bar"`
	// DevAtStated is development at the deployed stated bar.
	DevAtStated BarPoint  `json:"dev_at_stated_095"`
	DevAtMap    *BarPoint `json:"dev_at_map_bar,omitempty"`
	Candidate   bool      `json:"candidate"`
	// HeldOutApplied is true only for a candidate (D610).
	HeldOutApplied bool      `json:"held_out_applied"`
	HeldAtMap      *HeldBar  `json:"held_out_at_map_bar,omitempty"`
	HeldTable      []HeldBar `json:"held_out_stated_table"`
	// The map's held-out calibration beside stated confidence's, computed
	// once (D610), one bin per value.
	HeldECEStated float64 `json:"held_out_ece_stated"`
	HeldECEMap    float64 `json:"held_out_ece_map"`
	HeldN         int     `json:"held_out_n"`
	Adopted       bool    `json:"adopted"`
	Outcome       string  `json:"outcome"`
}

// LatRun is one run's latency.
type LatRun struct {
	Key        string    `json:"key"`
	File       string    `json:"file"`
	N          int       `json:"n"`
	P50MS      int64     `json:"latency_p50_ms"`
	P95MS      int64     `json:"latency_p95_ms"`
	OutTokP50  int       `json:"output_tokens_p50"`
	SpearmanLO float64   `json:"spearman_latency_output_tokens"`
	Slowest    []LatCall `json:"slowest"`
	// From D611 on: the whole call, retries and hedges included.
	WallP50MS int64   `json:"wall_p50_ms,omitempty"`
	WallP95MS int64   `json:"wall_p95_ms,omitempty"`
	Correct   int     `json:"correct"`
	OutTok    int     `json:"output_tokens"`
	Gateway   float64 `json:"gateway_cost_usd,omitempty"`
	Hedged    int     `json:"hedges_sent,omitempty"`
}

// LatCall is one slow call.
type LatCall struct {
	Ruling    string `json:"ruling"`
	LatencyMS int64  `json:"latency_ms"`
	OutTok    int    `json:"output_tokens"`
	InTok     int    `json:"input_tokens"`
}

// P102BarLatency is D611's outcome.
type P102BarLatency struct {
	Runs []LatRun `json:"runs"`
	// The re-run arms (nil until run).
	ArmA *LatRun `json:"arm_a_no_hedge,omitempty"`
	ArmB *LatRun `json:"arm_b_hedge_12s,omitempty"`
	// Per-ruling rank correlation of arm A's latency with D600's run of the
	// same prompt on the same rulings: high if slow calls follow the input.
	SpearmanAvsD600 *float64 `json:"spearman_arm_a_vs_d600,omitempty"`
	// Where arm A's four slowest calls ranked in D600's run (1 = slowest).
	ArmASlowestRankInD600 map[string]int `json:"arm_a_slowest_rank_in_d600,omitempty"`
	Passed                *bool          `json:"hedge_passed,omitempty"`
	Reasons               []string       `json:"hedge_rule,omitempty"`
}

// P102BarSummary is summary.json.
type P102BarSummary struct {
	Note    string         `json:"note"`
	Rule    P102BarRule    `json:"d610_bar"`
	Latency P102BarLatency `json:"d611_latency"`
}

func cmdP102Bar(args []string) error {
	fl := flag.NewFlagSet("p102bar", flag.ExitOnError)
	check := fl.Bool("check", false, "recompute and fail if the committed summary differs")
	_ = fl.Parse(args)
	sum := P102BarSummary{Note: "D610: a development-only auto-accept rule for usmca-v5 Part 102 answers, applied to the committed held-out rows only if it gains coverage on development. D611: the Part 102 latency tail from committed rows and a development re-run with and without a hedged request. Recompute: cd backend && go run ./cmd/tariffshift p102bar -check"}
	r, err := p102barRule()
	if err != nil {
		return err
	}
	sum.Rule = r
	l, err := p102barLatency()
	if err != nil {
		return err
	}
	sum.Latency = l
	sb, _ := json.MarshalIndent(sum, "", "  ")
	if err := writeOrCheckPlans(p102barOut, append(sb, '\n'), *check); err != nil {
		return err
	}
	fmt.Printf("p102bar: %s\n", r.Outcome)
	for _, x := range l.Runs {
		fmt.Printf("p102bar latency %s: p50 %d ms, p95 %d ms, rho(latency, output tokens) %.2f\n", x.Key, x.P50MS, x.P95MS, x.SpearmanLO)
	}
	return nil
}

// lastRows is the last row per ruling (a resumed run appends the re-run).
func lastRows(path string) ([]g102Row, error) {
	rs, err := readG102(path)
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	var out []g102Row
	for _, x := range rs {
		if i, ok := idx[x.Ruling]; ok {
			out[i] = x
			continue
		}
		idx[x.Ruling] = len(out)
		out = append(out, x)
	}
	return out, nil
}

func rowOK(x g102Row) bool { return x.Error == "" && x.Correct }

func barPoint(rows []g102Row, bar float64, score func(g102Row) float64) BarPoint {
	p := BarPoint{Bar: bar, N: len(rows)}
	for _, x := range rows {
		if x.Error == "" && score(x) >= bar {
			p.Auto++
			if rowOK(x) {
				p.Right++
			}
		}
	}
	if p.Auto > 0 {
		p.Accuracy = round4(float64(p.Right) / float64(p.Auto))
		lo, hi := wilson(p.Right, p.Auto)
		p.CILow, p.CIHigh = lo, hi
	}
	return p
}

func stated(x g102Row) float64 { return x.Confidence }

// curve is every distinct value of score on rows, as a bar, highest first,
// and the lowest bar that meets D610's target.
func curve(rows []g102Row, score func(g102Row) float64) ([]BarPoint, *float64) {
	seen := map[float64]bool{}
	var vals []float64
	for _, x := range rows {
		if v := score(x); !seen[v] {
			seen[v] = true
			vals = append(vals, v)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(vals)))
	var out []BarPoint
	var bar *float64
	for _, v := range vals {
		p := barPoint(rows, v, score)
		out = append(out, p)
		if p.Auto >= p102barMinAuto && float64(p.Right) >= p102barTarget*float64(p.Auto) {
			b := v
			bar = &b
		}
	}
	return out, bar
}

// fitMap is the cal-v2 procedure (eval/charts/build_accuracy.py
// isotonic_fit_min): adjacent stated values pooled, lowest first, into
// blocks of at least minBlock cases, a short last block joining the one
// below; then pool-adjacent-violators; capped.
func fitMap(rows []g102Row) []MapStep {
	type blk struct {
		lo, hi float64
		n, k   int
	}
	by := map[float64]*blk{}
	var levels []float64
	for _, x := range rows {
		if x.Error != "" {
			continue
		}
		b := by[x.Confidence]
		if b == nil {
			b = &blk{lo: x.Confidence, hi: x.Confidence}
			by[x.Confidence] = b
			levels = append(levels, x.Confidence)
		}
		b.n++
		if x.Correct {
			b.k++
		}
	}
	sort.Float64s(levels)
	var blocks []blk
	var cur *blk
	for _, c := range levels {
		if cur == nil {
			cur = &blk{lo: c}
		}
		cur.hi = c
		cur.n += by[c].n
		cur.k += by[c].k
		if cur.n >= p102barMinBlock {
			blocks = append(blocks, *cur)
			cur = nil
		}
	}
	if cur != nil {
		if len(blocks) > 0 {
			last := &blocks[len(blocks)-1]
			last.hi, last.n, last.k = cur.hi, last.n+cur.n, last.k+cur.k
		} else {
			blocks = append(blocks, *cur)
		}
	}
	var merged []blk
	for _, b := range blocks {
		merged = append(merged, b)
		for len(merged) > 1 {
			x, y := merged[len(merged)-1], merged[len(merged)-2]
			if float64(y.k)/float64(y.n) <= float64(x.k)/float64(x.n) {
				break
			}
			merged = append(merged[:len(merged)-2], blk{lo: y.lo, hi: x.hi, n: x.n + y.n, k: x.k + y.k})
		}
	}
	out := make([]MapStep, len(merged))
	for i, b := range merged {
		out[i] = MapStep{From: b.lo, To: b.hi, N: b.n, Correct: b.k, Calibrated: round4(math.Min(p102barCap, float64(b.k)/float64(b.n)))}
	}
	return out
}

func asCalMap(steps []MapStep) calibrate.Map {
	m := calibrate.Map{}
	for _, s := range steps {
		m.Steps = append(m.Steps, calibrate.Step{From: s.From, Calibrated: s.Calibrated, N: s.N, Correct: s.Correct})
	}
	return m
}

func heldBar(engine string, w [2][]g102Row, bar float64, score func(g102Row) float64) HeldBar {
	pooled := append(append([]g102Row{}, w[0]...), w[1]...)
	return HeldBar{Engine: engine, Bar: bar, Window1: barPoint(w[0], bar, score), Window2: barPoint(w[1], bar, score), Pooled: barPoint(pooled, bar, score)}
}

func p102barRule() (P102BarRule, error) {
	r := P102BarRule{Target: p102barTarget, MinAuto: p102barMinAuto, DevFile: trimDots(p102barDev)}
	dev, err := lastRows(p102barDev)
	if err != nil {
		return r, err
	}
	devV1, err := lastRows(p102barDevV1)
	if err != nil {
		return r, err
	}
	r.DevN = len(dev)
	for _, x := range dev {
		if rowOK(x) {
			r.DevCorrect++
		}
	}
	r.StatedCurve, r.StatedRuleBar = curve(dev, stated)
	r.DevV1Curve, _ = curve(devV1, stated)
	r.Map = fitMap(dev)
	cm := asCalMap(r.Map)
	mapped := func(x g102Row) float64 { return cm.Value(x.Confidence) }
	r.MapCurve, r.MapBar = curve(dev, mapped)
	r.DevAtStated = barPoint(dev, p102barStated, stated)
	if r.MapBar != nil {
		p := barPoint(dev, *r.MapBar, mapped)
		r.DevAtMap = &p
		r.Candidate = r.DevAtMap.Auto >= p102barMinAuto && r.DevAtMap.Auto > r.DevAtStated.Auto
	}

	held := map[string][2][]g102Row{}
	for _, eng := range []string{"usmca-v1", "usmca-v5"} {
		var w [2][]g102Row
		for i, f := range p102barHeld[eng] {
			if w[i], err = lastRows(f); err != nil {
				return r, err
			}
		}
		held[eng] = w
		for _, b := range []float64{0.90, 0.92, 0.95, 0.96, 0.98} {
			r.HeldTable = append(r.HeldTable, heldBar(eng, w, b, stated))
		}
	}
	v5 := held["usmca-v5"]
	before := heldBar("usmca-v5", v5, p102barStated, stated).Pooled
	var scored []calibrate.Scored
	for _, w := range v5 {
		for _, x := range w {
			scored = append(scored, calibrate.Scored{Stated: x.Confidence, OK: rowOK(x)})
		}
	}
	ev := cm.Evaluate(scored)
	r.HeldN, r.HeldECEStated, r.HeldECEMap = ev.N, round4(ev.ECEStated), round4(ev.ECECalibrated)
	switch {
	case !r.Candidate && r.MapBar == nil:
		r.Outcome = fmt.Sprintf("no candidate: no step of the map (highest %.4f) is at least %.0f%% right on at least %d development cases; the bar stays stated %.2f (development %d of %d auto-accepted, %d right)",
			r.Map[len(r.Map)-1].Calibrated, 100*p102barTarget, p102barMinAuto, p102barStated, r.DevAtStated.Auto, r.DevN, r.DevAtStated.Right)
	case !r.Candidate:
		r.Outcome = fmt.Sprintf("no candidate: the map's bar auto-accepts %d development cases against %d at stated %.2f; the bar stays stated %.2f", r.DevAtMap.Auto, r.DevAtStated.Auto, p102barStated, p102barStated)
	default:
		r.HeldOutApplied = true
		h := heldBar("usmca-v5", v5, *r.MapBar, mapped)
		r.HeldAtMap = &h
		r.Adopted = h.Pooled.Auto > before.Auto && float64(h.Pooled.Right) >= p102barTarget*float64(h.Pooled.Auto)
		verdict := "not adopted: the bar stays stated 0.95"
		if r.Adopted {
			verdict = "adopted"
		}
		r.Outcome = fmt.Sprintf("candidate; held out %d of %d auto-accepted, %d right, against %d and %d at stated %.2f: %s", h.Pooled.Auto, h.Pooled.N, h.Pooled.Right, before.Auto, before.Right, p102barStated, verdict)
	}
	return r, nil
}

// spearman is the rank correlation of a and b (average ranks for ties).
func spearman(a, b []float64) float64 {
	ra, rb := ranks(a), ranks(b)
	n := float64(len(a))
	var ma, mb float64
	for i := range ra {
		ma += ra[i]
		mb += rb[i]
	}
	ma, mb = ma/n, mb/n
	var sab, saa, sbb float64
	for i := range ra {
		sab += (ra[i] - ma) * (rb[i] - mb)
		saa += (ra[i] - ma) * (ra[i] - ma)
		sbb += (rb[i] - mb) * (rb[i] - mb)
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

func ranks(x []float64) []float64 {
	idx := make([]int, len(x))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool { return x[idx[i]] < x[idx[j]] })
	r := make([]float64, len(x))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && x[idx[j+1]] == x[idx[i]] {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			r[idx[k]] = avg
		}
		i = j + 1
	}
	return r
}

func latRun(key, path string) (LatRun, []g102Row, error) {
	rows, err := lastRows(path)
	if err != nil {
		return LatRun{}, nil, err
	}
	out := LatRun{Key: key, File: trimDots(path), N: len(rows)}
	var lat, wall []int64
	var tok []int
	var fl, ft []float64
	for _, x := range rows {
		out.OutTok += x.OutTok
		out.Gateway += x.Gateway
		if rowOK(x) {
			out.Correct++
		}
		if x.WallMS > 0 {
			wall = append(wall, x.WallMS)
			if x.WallMS >= p102barHedgeMS {
				out.Hedged++
			}
		}
		if x.LatencyMS > 0 {
			lat = append(lat, x.LatencyMS)
			tok = append(tok, x.OutTok)
			fl, ft = append(fl, float64(x.LatencyMS)), append(ft, float64(x.OutTok))
		}
	}
	out.Gateway = round4(out.Gateway)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	sort.Ints(tok)
	if len(lat) > 0 {
		out.P50MS, out.P95MS = pctl(lat, 0.50), pctl(lat, 0.95)
		out.OutTokP50 = tok[max(0, int(math.Ceil(0.5*float64(len(tok))))-1)]
		out.SpearmanLO = round4(spearman(fl, ft))
	}
	if len(wall) > 0 {
		sort.Slice(wall, func(i, j int) bool { return wall[i] < wall[j] })
		out.WallP50MS, out.WallP95MS = pctl(wall, 0.50), pctl(wall, 0.95)
	}
	slow := append([]g102Row{}, rows...)
	sort.SliceStable(slow, func(i, j int) bool { return slow[i].LatencyMS > slow[j].LatencyMS })
	for _, x := range slow[:min(3, len(slow))] {
		out.Slowest = append(out.Slowest, LatCall{Ruling: x.Ruling, LatencyMS: x.LatencyMS, OutTok: x.OutTok, InTok: x.InTok})
	}
	return out, rows, nil
}

func p102barLatency() (P102BarLatency, error) {
	var l P102BarLatency
	runs := []struct{ key, file string }{
		{"dev_usmca_v1_d550", p102barDevV1},
		{"dev_usmca_v5_d600", p102barDev},
		{"window1_usmca_v1", p102barHeld["usmca-v1"][0]},
		{"window1_usmca_v5", p102barHeld["usmca-v5"][0]},
		{"window2_usmca_v1", p102barHeld["usmca-v1"][1]},
		{"window2_usmca_v5", p102barHeld["usmca-v5"][1]},
	}
	var d600 []g102Row
	for _, r := range runs {
		x, rows, err := latRun(r.key, r.file)
		if err != nil {
			return l, err
		}
		if r.file == p102barDev {
			d600 = rows
		}
		l.Runs = append(l.Runs, x)
	}
	a, rowsA, errA := latRun("arm_a_no_hedge", p102barArmA)
	b, _, errB := latRun("arm_b_hedge_12s", p102barArmB)
	if os.IsNotExist(errA) || os.IsNotExist(errB) {
		return l, nil // not run yet
	}
	if errA != nil {
		return l, errA
	}
	if errB != nil {
		return l, errB
	}
	l.ArmA, l.ArmB = &a, &b
	byD600 := map[string]int64{}
	for _, x := range d600 {
		byD600[x.Ruling] = x.LatencyMS
	}
	var xa, xd []float64
	for _, x := range rowsA {
		if d, ok := byD600[x.Ruling]; ok && x.LatencyMS > 0 && d > 0 {
			xa, xd = append(xa, float64(x.LatencyMS)), append(xd, float64(d))
		}
	}
	rho := round4(spearman(xa, xd))
	l.SpearmanAvsD600 = &rho
	order := append([]g102Row{}, d600...)
	sort.SliceStable(order, func(i, j int) bool { return order[i].LatencyMS > order[j].LatencyMS })
	rank := map[string]int{}
	for i, x := range order {
		rank[x.Ruling] = i + 1
	}
	slowA := append([]g102Row{}, rowsA...)
	sort.SliceStable(slowA, func(i, j int) bool { return slowA[i].LatencyMS > slowA[j].LatencyMS })
	l.ArmASlowestRankInD600 = map[string]int{}
	for _, x := range slowA[:min(4, len(slowA))] {
		l.ArmASlowestRankInD600[x.Ruling] = rank[x.Ruling]
	}
	// D611's rule for proposing the hedge.
	p95 := float64(b.WallP95MS) <= 0.8*float64(a.WallP95MS)
	acc := b.Correct >= a.Correct-3
	cost := a.Gateway > 0 && b.Gateway <= 1.3*a.Gateway
	l.Reasons = []string{
		fmt.Sprintf("95th-percentile wall time %d ms against %d ms (at most 80%%: %v)", b.WallP95MS, a.WallP95MS, p95),
		fmt.Sprintf("correct %d against %d (at least %d: %v)", b.Correct, a.Correct, a.Correct-3, acc),
		fmt.Sprintf("gateway cost $%.4f against $%.4f (at most 1.3 x: %v)", b.Gateway, a.Gateway, cost),
	}
	passed := p95 && acc && cost
	l.Passed = &passed
	return l, nil
}
