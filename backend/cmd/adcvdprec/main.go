// Command adcvdprec scores refinements of the AD/CVD exposure check that aim
// at precision (D942, D943): the same scoring as adcvdeval, for several
// methods side by side on one set of Commerce scope rulings.
//
//	go run ./cmd/adcvdprec -dir eval/adcvd/dev -orders /tmp/orders-2021-12-31.json -methods base,scope,reader -reader eval/adcvd/dev/reader.jsonl
//	go run ./cmd/adcvdprec -dir ... -orders ... -pairs pairs.jsonl     # write what the scope reader must be asked
//
// The scope reader (a model, see eval/adcvd/reader.py) is never called here:
// its recorded answers are read from a file, so a score recomputes offline.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
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
	Title        string   `json:"title"`
	OrderCountry string   `json:"order_country"`
	RulingDate   string   `json:"ruling_date"`
}

type label struct {
	ID            string `json:"id"`
	Outcome       string `json:"outcome"`
	TurnsOnOrigin bool   `json:"turns_on_origin"`
	Description   string `json:"description"`
	HTS           string `json:"hts"`
}

// Flag is whether a method flags the ruling's own order and any order.
type Flag struct {
	Own bool `json:"own"`
	Any bool `json:"any"`
}

// Row is one scored ruling.
type Row struct {
	ID          string          `json:"id"`
	Cases       []string        `json:"cases"`
	Outcome     string          `json:"outcome"`
	HTS         string          `json:"hts"`
	Origin      string          `json:"origin"`
	SnapshotGap bool            `json:"order_not_in_snapshot"`
	Flags       map[string]Flag `json:"flags"`
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

// MethodSummary is one method's figures.
type MethodSummary struct {
	Own Confusion `json:"own_order"`
	Any Confusion `json:"any_order"`
	// TPKept is the share of the baseline's true positives (own order) this method keeps.
	TPKept Rate `json:"true_positives_kept_of_base"`
	// RemovedFP and RemovedTP count the baseline's flags (own order) the method removes, by what the ruling found.
	RemovedFP int `json:"removed_false_positives"`
	RemovedTP int `json:"removed_true_positives"`
}

// Summary is what a run reports.
type Summary struct {
	Snapshot       string                   `json:"snapshot"`
	SnapshotOrders int                      `json:"snapshot_orders"`
	Rulings        int                      `json:"rulings"`
	Scored         int                      `json:"scored"`
	Within         int                      `json:"within"`
	Outside        int                      `json:"outside"`
	Skipped        map[string]int           `json:"skipped"`
	OrderNotInSnap int                      `json:"within_on_orders_not_in_snapshot"`
	ReaderMissing  int                      `json:"reader_answers_missing,omitempty"`
	Methods        map[string]MethodSummary `json:"methods"`
	FlagEvery      Confusion                `json:"baseline_flag_every_ruling"`
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

func pairKey(o adcvd.Order, name string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", o.Product, o.Country, o.ScopeText, name)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// fileReader answers from a file of recorded verdicts, and notes every
// question it is asked.
type fileReader struct {
	verdicts map[string]adcvd.Verdict
	asked    map[string]Pair
	missing  map[string]bool
	cur      string
}

func (f *fileReader) Read(p adcvd.Part, o adcvd.Order) (adcvd.Verdict, error) {
	k := pairKey(o, p.Name)
	f.asked[k] = Pair{Key: k, ID: f.cur, Cases: o.Case, Product: o.Product, Country: o.Country, Description: p.Name, ScopeText: o.ScopeText}
	v, ok := f.verdicts[k]
	if !ok {
		f.missing[k] = true
		return "", nil
	}
	return v, nil
}

func presets(r adcvd.Reader) map[string]adcvd.Config {
	return map[string]adcvd.Config{
		"base":         {},
		"hts6":         {MinListedDigits: 6},
		"hts8":         {MinListedDigits: 8},
		"title":        {TitleTerms: true},
		"scope":        {ScopeTerms: true},
		"excl":         {Exclusions: true},
		"scope+excl":   {ScopeTerms: true, Exclusions: true},
		"reader":       {Reader: r},
		"lean":         {Reader: r, ReaderLean: true},
		"scope+lean":   {ScopeTerms: true, Reader: r, ReaderLean: true},
		"scope+reader": {ScopeTerms: true, Reader: r},
		"excl+reader":  {Exclusions: true, Reader: r},
		"all":          {ScopeTerms: true, Exclusions: true, Reader: r},
	}
}

func main() {
	dir := flag.String("dir", "", "directory with rulings.jsonl and labels.jsonl")
	orders := flag.String("orders", "", "orders snapshot")
	methods := flag.String("methods", "base", "comma separated method names")
	readerFile := flag.String("reader", "", "recorded scope reader answers (reader.py output)")
	readerModel := flag.String("reader-model", "", "use only answers of this model")
	readerVersion := flag.String("reader-version", "", "use only answers made with this prompt version")
	pairs := flag.String("pairs", "", "write the questions the scope reader must be asked to this file and exit")
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
	fr := &fileReader{verdicts: map[string]adcvd.Verdict{}, asked: map[string]Pair{}, missing: map[string]bool{}}
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
				v := adcvd.Verdict(a.Verdict)
				if v == adcvd.VerdictUnclear && a.Lean != "" {
					v = adcvd.Verdict("unclear_" + a.Lean)
				}
				fr.verdicts[a.Key] = v
			}
		}
	}
	all := presets(fr)
	var names []string
	for _, m := range strings.Split(*methods, ",") {
		m = strings.TrimSpace(m)
		if _, ok := all[m]; !ok {
			must(fmt.Errorf("unknown method %q", m))
		}
		names = append(names, m)
	}
	if *pairs != "" {
		names = []string{"reader"}
	}
	rows, s := score(t, rulings, labels, names, all, fr)
	if *pairs != "" {
		f, err := os.Create(*pairs)
		must(err)
		var keys []string
		for k := range fr.asked {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b, _ := json.Marshal(fr.asked[k])
			fmt.Fprintln(f, string(b))
		}
		f.Close()
		fmt.Println("pairs:", len(keys))
		return
	}
	s.ReaderMissing = len(fr.missing)
	b, _ := json.MarshalIndent(s, "", " ")
	b = append(b, '\n')
	if *check {
		have, err := os.ReadFile(*sum)
		must(err)
		if string(have) != string(b) {
			fmt.Fprintln(os.Stderr, "summary differs from a recomputation")
			os.Exit(1)
		}
		fmt.Println("adcvdprec: summary reproduced,", s.Scored, "rulings scored")
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
	fmt.Print(table(s, names))
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

func score(t *adcvd.Table, rulings []ruling, labels map[string]label, names []string, all map[string]adcvd.Config, fr *fileReader) ([]Row, Summary) {
	s := Summary{Snapshot: t.Snapshot(), SnapshotOrders: len(t.Orders()), Rulings: len(rulings), Skipped: map[string]int{}, Methods: map[string]MethodSummary{}}
	var rows []Row
	for _, r := range rulings {
		l, ok := labels[r.ID]
		switch {
		case !ok:
			s.Skipped["no_label"]++
			continue
		case l.Outcome != "within" && l.Outcome != "outside":
			s.Skipped["outcome_"+l.Outcome]++
			continue
		case strings.TrimSpace(l.HTS) == "":
			s.Skipped["no_hts"]++
			continue
		}
		row := Row{ID: r.ID, Cases: r.Cases, Outcome: l.Outcome, HTS: l.HTS, Origin: r.OrderCountry, SnapshotGap: !hasCase(t, r.Cases), Flags: map[string]Flag{}}
		part := adcvd.Part{Name: l.Description, HTS: l.HTS, Origin: r.OrderCountry}
		fr.cur = r.ID
		skipped := false
		for _, name := range names {
			res := t.CheckWith(adcvd.Input{Parts: []adcvd.Part{part}}, all[name])
			p := res.Parts[0]
			if p.Status == adcvd.StatusNotChecked {
				s.Skipped["not_checked"]++
				skipped = true
				break
			}
			var f Flag
			f.Any = p.Status == adcvd.StatusPossible
			for _, m := range p.Matches {
				for _, c := range m.Cases {
					for _, rc := range r.Cases {
						if c == rc {
							f.Own = true
						}
					}
				}
			}
			row.Flags[name] = f
		}
		if !skipped {
			rows = append(rows, row)
		}
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
	s.FlagEvery = confusion(rows, func(Row) bool { return true })
	base := confusion(rows, func(r Row) bool { return r.Flags["base"].Own })
	for _, name := range names {
		ms := MethodSummary{
			Own: confusion(rows, func(r Row) bool { return r.Flags[name].Own }),
			Any: confusion(rows, func(r Row) bool { return r.Flags[name].Any }),
		}
		for _, r := range rows {
			if r.Flags["base"].Own && !r.Flags[name].Own {
				if r.Outcome == "within" {
					ms.RemovedTP++
				} else {
					ms.RemovedFP++
				}
			}
		}
		ms.TPKept = wilson(ms.Own.TP, base.TP)
		s.Methods[name] = ms
	}
	return rows, s
}

func table(s Summary, names []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "snapshot %s (%d orders): %d rulings, %d scored (%d within, %d outside), skipped %v, reader answers missing %d\n", s.Snapshot, s.SnapshotOrders, s.Rulings, s.Scored, s.Within, s.Outside, s.Skipped, s.ReaderMissing)
	fmt.Fprintf(&b, "flag every ruling: precision %d/%d\n", s.FlagEvery.TP, s.FlagEvery.TP+s.FlagEvery.FP)
	fmt.Fprintf(&b, "%-14s %8s %8s  %-22s %-22s  %s\n", "method", "TP/FP", "FN/TN", "precision(own)", "recall(own)", "removed FP/TP; any-order precision")
	for _, n := range names {
		m := s.Methods[n]
		fmt.Fprintf(&b, "%-14s %3d/%-4d %3d/%-4d  %2d/%-2d %5.1f%% [%4.1f-%4.1f]  %2d/%-2d %5.1f%% [%4.1f-%4.1f]  %d/%d  any %d/%d\n", n, m.Own.TP, m.Own.FP, m.Own.FN, m.Own.TN,
			m.Own.Precision.K, m.Own.Precision.N, 100*m.Own.Precision.P, 100*m.Own.Precision.Lo, 100*m.Own.Precision.Hi,
			m.Own.Recall.K, m.Own.Recall.N, 100*m.Own.Recall.P, 100*m.Own.Recall.Lo, 100*m.Own.Recall.Hi,
			m.RemovedFP, m.RemovedTP, m.Any.Precision.K, m.Any.Precision.N)
	}
	return b.String()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adcvdprec:", err)
		os.Exit(1)
	}
}
