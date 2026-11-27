// Command adcvdscope scores the scope-first AD/CVD exposure method (D944):
// orders of the product's country are retrieved by the words of an importer's
// product description, a model reads each candidate's written scope against
// the description, and an order is flagged when the reader says in or unclear.
//
//	go run ./cmd/adcvdscope -dir eval/adcvd/dev -orders eval/adcvd/orders-2021-12-31.json -desc eval/adcvd/dev/descriptions.jsonl -k 8 -pairs pairs-v4.jsonl
//	go run ./cmd/adcvdscope -dir ... -orders ... -desc ... -k 8 -reader reader-v4.jsonl -reader-model M -reader-version V -summary summary-v4.json
//
// The reader (eval/adcvd/scope_reader.py) is never called here: its recorded
// answers are read from a file, so a score recomputes offline.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
)

type ruling struct {
	ID           string   `json:"id"`
	Cases        []string `json:"cases"`
	OrderCountry string   `json:"order_country"`
}

type label struct {
	ID            string `json:"id"`
	Outcome       string `json:"outcome"`
	TurnsOnOrigin bool   `json:"turns_on_origin"`
	HTS           string `json:"hts"`
}

type desc struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	HTS         string `json:"hts"`
}

// Rate is a count out of a total with its Wilson 95% interval.
type Rate struct {
	K  int     `json:"k"`
	N  int     `json:"n"`
	P  float64 `json:"p"`
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

// Confusion is the scoring of one flag against the outcomes.
type Confusion struct {
	TP, FP, FN, TN int
	Precision      Rate
	Recall         Rate
	Specificity    Rate
}

// Pair is one question for the scope reader.
type Pair struct {
	Key         string `json:"key"`
	ID          string `json:"id"`
	Cases       string `json:"cases"`
	Product     string `json:"product"`
	Country     string `json:"country"`
	Description string `json:"description"`
	ScopeText   string `json:"scope_text"`
}

// Cand is one retrieved candidate of a ruling.
type Cand struct {
	Cases   []string `json:"cases"`
	Key     string   `json:"key"`
	Own     bool     `json:"own"`
	Verdict string   `json:"verdict,omitempty"`
}

// Flag is whether a policy flags the ruling's own order and any order.
type Flag struct {
	Own bool `json:"own"`
	Any bool `json:"any"`
}

// Row is one scored ruling.
type Row struct {
	ID          string          `json:"id"`
	Outcome     string          `json:"outcome"`
	Origin      string          `json:"origin"`
	SnapshotGap bool            `json:"order_not_in_snapshot"`
	OwnKey      string          `json:"own_key,omitempty"`     // the question for the ruling's own order, retrieved or not
	OwnVerdict  string          `json:"own_verdict,omitempty"` // the reader's answer to it
	OwnRank     int             `json:"own_rank"`              // 1-based rank of the ruling's own order among the candidates, 0 if not retrieved
	Cands       []Cand          `json:"candidates"`
	Flags       map[string]Flag `json:"flags"`
}

// PolicySummary is one flag policy's figures.
type PolicySummary struct {
	Own Confusion `json:"own_order"`
	Any Confusion `json:"any_order"`
}

// Summary is what a run reports.
type Summary struct {
	Snapshot       string                    `json:"snapshot"`
	SnapshotOrders int                       `json:"snapshot_orders"`
	K              int                       `json:"k"`
	Rulings        int                       `json:"rulings"`
	Scored         int                       `json:"scored"`
	Within         int                       `json:"within"`
	Outside        int                       `json:"outside"`
	Skipped        map[string]int            `json:"skipped"`
	OrderNotInSnap int                       `json:"within_on_orders_not_in_snapshot"`
	OwnInTopK      Rate                      `json:"within_own_order_retrieved"`
	OwnInTop1      Rate                      `json:"within_own_order_rank1"`
	ReaderMissing  int                       `json:"reader_answers_missing,omitempty"`
	Verdicts       map[string]map[string]int `json:"own_order_verdicts_by_outcome,omitempty"`
	Policies       map[string]PolicySummary  `json:"policies"`
	ReaderOnly     map[string]Confusion      `json:"reader_only_own_order,omitempty"` // the reader's answer on the own order, retrieval aside
	FlagEvery      Confusion                 `json:"baseline_flag_every_ruling"`
}

func round6(x float64) float64 { return math.Round(x*1e6) / 1e6 }

func wilson(k, n int) Rate {
	r := Rate{K: k, N: n}
	if n == 0 {
		return r
	}
	const z = 1.959964
	p := float64(k) / float64(n)
	d := 1 + z*z/float64(n)
	c := p + z*z/(2*float64(n))
	h := z * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n)))
	r.P, r.Lo, r.Hi = round6(p), round6(math.Max(0, (c-h)/d)), round6(math.Min(1, (c+h)/d))
	return r
}

func confusion(rows []Row, flag func(Row) bool) Confusion {
	var c Confusion
	for _, r := range rows {
		pos, f := r.Outcome == "within", flag(r)
		switch {
		case pos && f:
			c.TP++
		case !pos && f:
			c.FP++
		case pos && !f:
			c.FN++
		default:
			c.TN++
		}
	}
	c.Precision = wilson(c.TP, c.TP+c.FP)
	c.Recall = wilson(c.TP, c.TP+c.FN)
	c.Specificity = wilson(c.TN, c.TN+c.FP)
	return c
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// policies are the ways the reader's answers become a flag on a candidate.
var policies = map[string]func(v string) bool{
	"in":            func(v string) bool { return v == "in" },
	"in+unclear":    func(v string) bool { return v == "in" || v == "unclear" || v == "unclear_in" || v == "unclear_out" },
	"in+lean_in":    func(v string) bool { return v == "in" || v == "unclear_in" },
	"retrieve_only": func(v string) bool { return true },
}

// PolicyNames lists the policies in report order.
var PolicyNames = []string{"retrieve_only", "in+unclear", "in+lean_in", "in"}

type fileReader struct {
	verdicts map[string]string
	missing  map[string]bool
}

func (f *fileReader) verdict(key string) string {
	v, ok := f.verdicts[key]
	if !ok {
		f.missing[key] = true
		return ""
	}
	return v
}

func hasCase(t *adcvd.Table, cases []string) bool {
	for _, o := range t.Orders() {
		for _, c := range cases {
			if o.Case == c {
				return true
			}
		}
	}
	return false
}

func key(c adcvd.Candidate, d string) string {
	return adcvd.PairKey(adcvd.Order{Product: c.Product, Country: c.Country, ScopeText: c.Scope}, d)
}

// score retrieves the candidates of every scorable ruling and applies the policies. useHTS passes the
// description file's HTS code as a hint. The pairs the reader must be asked are returned.
func score(t *adcvd.Table, rulings []ruling, labels map[string]label, descs map[string]desc, k int, useHTS bool, fr *fileReader) ([]Row, Summary, map[string]Pair) {
	s := Summary{Snapshot: t.Snapshot(), SnapshotOrders: len(t.Orders()), K: k, Rulings: len(rulings), Skipped: map[string]int{}, Policies: map[string]PolicySummary{}, Verdicts: map[string]map[string]int{}}
	pairs := map[string]Pair{}
	var rows []Row
	for _, r := range rulings {
		l, ok := labels[r.ID]
		d := descs[r.ID]
		switch {
		case !ok:
			s.Skipped["no_label"]++
			continue
		case l.Outcome != "within" && l.Outcome != "outside":
			s.Skipped["outcome_"+l.Outcome]++
			continue
		case strings.TrimSpace(d.Description) == "":
			s.Skipped["no_description"]++
			continue
		}
		q := adcvd.Query{Description: d.Description, Country: r.OrderCountry}
		if useHTS {
			q.HTS = d.HTS
		}
		cands := t.Candidates(q, k)
		row := Row{ID: r.ID, Outcome: l.Outcome, Origin: r.OrderCountry, SnapshotGap: !hasCase(t, r.Cases), Flags: map[string]Flag{}}
		for i, c := range cands {
			cd := Cand{Cases: c.Cases, Key: key(c, d.Description)}
			for _, cc := range c.Cases {
				for _, rc := range r.Cases {
					if cc == rc {
						cd.Own = true
					}
				}
			}
			if cd.Own && row.OwnRank == 0 {
				row.OwnRank = i + 1
			}
			pairs[cd.Key] = Pair{Key: cd.Key, ID: r.ID, Cases: strings.Join(c.Cases, " "), Product: c.Product, Country: c.Country, Description: d.Description, ScopeText: c.Scope}
			cd.Verdict = fr.verdict(cd.Key)
			row.Cands = append(row.Cands, cd)
		}
		if g, ok := t.GroupOf(r.Cases); ok {
			row.OwnKey = key(g, d.Description)
			pairs[row.OwnKey] = Pair{Key: row.OwnKey, ID: r.ID, Cases: strings.Join(g.Cases, " "), Product: g.Product, Country: g.Country, Description: d.Description, ScopeText: g.Scope}
			row.OwnVerdict = fr.verdict(row.OwnKey)
		}
		rows = append(rows, row)
	}
	s.Scored = len(rows)
	for _, r := range rows {
		if r.Outcome == "within" {
			s.Within++
			if r.SnapshotGap {
				s.OrderNotInSnap++
			}
		} else {
			s.Outside++
		}
	}
	for i := range rows {
		r := &rows[i]
		for _, name := range PolicyNames {
			f := Flag{}
			for _, c := range r.Cands {
				if c.Verdict == "" && name != "retrieve_only" {
					continue
				}
				if policies[name](c.Verdict) {
					f.Any = true
					if c.Own {
						f.Own = true
					}
				}
			}
			r.Flags[name] = f
		}
		for _, c := range r.Cands {
			if c.Own && c.Verdict != "" {
				if s.Verdicts[r.Outcome] == nil {
					s.Verdicts[r.Outcome] = map[string]int{}
				}
				s.Verdicts[r.Outcome][c.Verdict]++
			}
		}
	}
	w, w1 := 0, 0
	for _, r := range rows {
		if r.Outcome == "within" {
			if r.OwnRank > 0 {
				w++
			}
			if r.OwnRank == 1 {
				w1++
			}
		}
	}
	s.OwnInTopK, s.OwnInTop1 = wilson(w, s.Within), wilson(w1, s.Within)
	s.FlagEvery = confusion(rows, func(Row) bool { return true })
	for _, name := range PolicyNames {
		n := name
		s.Policies[n] = PolicySummary{
			Own: confusion(rows, func(r Row) bool { return r.Flags[n].Own }),
			Any: confusion(rows, func(r Row) bool { return r.Flags[n].Any }),
		}
	}
	if len(fr.verdicts) > 0 {
		s.ReaderOnly = map[string]Confusion{}
		for _, name := range PolicyNames[1:] {
			n := name
			s.ReaderOnly[n] = confusion(rows, func(r Row) bool { return r.OwnVerdict != "" && policies[n](r.OwnVerdict) })
		}
	}
	return rows, s, pairs
}

func main() {
	dir := flag.String("dir", "", "directory with rulings.jsonl and labels.jsonl")
	orders := flag.String("orders", "", "orders snapshot")
	descFile := flag.String("desc", "", "descriptions file (id, description, optional hts)")
	k := flag.Int("k", 8, "candidates per ruling")
	useHTS := flag.Bool("hint", false, "pass the HTS code of the descriptions file as a retrieval hint")
	readerFile := flag.String("reader", "", "recorded scope reader answers")
	readerModel := flag.String("reader-model", "", "use only answers of this model")
	readerVersion := flag.String("reader-version", "", "use only answers made with this prompt version")
	pairs := flag.String("pairs", "", "write the questions the reader must be asked to this file")
	out := flag.String("out", "", "per-ruling results")
	sum := flag.String("summary", "", "summary")
	check := flag.Bool("check", false, "recompute and fail if the summary on disk differs")
	flag.Parse()
	t, err := adcvd.LoadFile(*orders)
	must(err)
	rulings, err := readJSONL[ruling](*dir + "/rulings.jsonl")
	must(err)
	ls, err := readJSONL[label](*dir + "/labels.jsonl")
	must(err)
	labels := map[string]label{}
	for _, l := range ls {
		labels[l.ID] = l
	}
	ds, err := readJSONL[desc](*descFile)
	must(err)
	descs := map[string]desc{}
	for _, d := range ds {
		descs[d.ID] = d
	}
	fr := &fileReader{verdicts: map[string]string{}, missing: map[string]bool{}}
	if *readerFile != "" {
		raw, err := readJSONL[struct {
			Key     string `json:"key"`
			Model   string `json:"model"`
			Version string `json:"prompt_version"`
			Verdict string `json:"verdict"`
			Lean    string `json:"lean"`
		}](*readerFile)
		must(err)
		for _, a := range raw {
			if (*readerModel == "" || a.Model == *readerModel) && (*readerVersion == "" || a.Version == *readerVersion) {
				v := a.Verdict
				if v == "unclear" && a.Lean != "" {
					v = "unclear_" + a.Lean
				}
				fr.verdicts[a.Key] = v
			}
		}
	}
	rows, s, ps := score(t, rulings, labels, descs, *k, *useHTS, fr)
	if *pairs != "" {
		f, err := os.Create(*pairs)
		must(err)
		var keys []string
		for kk := range ps {
			keys = append(keys, kk)
		}
		sort.Strings(keys)
		for _, kk := range keys {
			b, _ := json.Marshal(ps[kk])
			fmt.Fprintln(f, string(b))
		}
		f.Close()
		fmt.Println("pairs:", len(keys))
	}
	if *readerFile != "" {
		s.ReaderMissing = len(fr.missing)
	}
	b, _ := json.MarshalIndent(s, "", " ")
	b = append(b, '\n')
	if *check {
		have, err := os.ReadFile(*sum)
		must(err)
		if string(have) != string(b) {
			fmt.Fprintln(os.Stderr, "summary differs from a recomputation")
			os.Exit(1)
		}
		fmt.Println("adcvdscope: summary reproduced,", s.Scored, "rulings scored")
		return
	}
	if *out != "" {
		f, err := os.Create(*out)
		must(err)
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		for _, r := range rows {
			bb, _ := json.Marshal(r)
			fmt.Fprintln(f, string(bb))
		}
		f.Close()
	}
	if *sum != "" {
		must(os.WriteFile(*sum, b, 0o644))
	}
	fmt.Print(table(s))
}

func table(s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "snapshot %s (%d orders), K=%d: %d rulings, %d scored (%d within, %d outside), skipped %v, reader answers missing %d\n", s.Snapshot, s.SnapshotOrders, s.K, s.Rulings, s.Scored, s.Within, s.Outside, s.Skipped, s.ReaderMissing)
	fmt.Fprintf(&b, "within rulings: order not in list %d; own order retrieved %d/%d (%.1f%%), rank 1 %d/%d\n", s.OrderNotInSnap, s.OwnInTopK.K, s.OwnInTopK.N, 100*s.OwnInTopK.P, s.OwnInTop1.K, s.OwnInTop1.N)
	fmt.Fprintf(&b, "flag every ruling: precision %d/%d\n", s.FlagEvery.TP, s.FlagEvery.TP+s.FlagEvery.FP)
	fmt.Fprintf(&b, "own-order verdicts by outcome: %v\n", s.Verdicts)
	for _, n := range PolicyNames[1:] {
		if m, ok := s.ReaderOnly[n]; ok {
			fmt.Fprintf(&b, "reader-only(own order, no retrieval) %-11s precision %d/%d %.1f%%  recall %d/%d %.1f%%\n", n, m.Precision.K, m.Precision.N, 100*m.Precision.P, m.Recall.K, m.Recall.N, 100*m.Recall.P)
		}
	}
	fmt.Fprintf(&b, "%-14s %8s %8s  %-22s %-22s  %s\n", "policy", "TP/FP", "FN/TN", "precision(own)", "recall(own)", "any-order precision/recall")
	for _, n := range PolicyNames {
		m := s.Policies[n]
		fmt.Fprintf(&b, "%-14s %3d/%-4d %3d/%-4d  %2d/%-2d %5.1f%% [%4.1f-%4.1f]  %2d/%-2d %5.1f%% [%4.1f-%4.1f]  any %d/%d %d/%d\n", n, m.Own.TP, m.Own.FP, m.Own.FN, m.Own.TN,
			m.Own.Precision.K, m.Own.Precision.N, 100*m.Own.Precision.P, 100*m.Own.Precision.Lo, 100*m.Own.Precision.Hi,
			m.Own.Recall.K, m.Own.Recall.N, 100*m.Own.Recall.P, 100*m.Own.Recall.Lo, 100*m.Own.Recall.Hi,
			m.Any.Precision.K, m.Any.Precision.N, m.Any.Recall.K, m.Any.Recall.N)
	}
	return b.String()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adcvdscope:", err)
		os.Exit(1)
	}
}
