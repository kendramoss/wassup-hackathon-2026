// Command adcvdeval scores the AD/CVD exposure check against Commerce scope
// rulings (D940).
//
//	go run ./cmd/adcvdeval                       # held-out run: the 2024-12-31 snapshot
//	go run ./cmd/adcvdeval -orders backend/internal/adcvd/data/orders.json -out eval/adcvd/results-shipped.jsonl -summary eval/adcvd/summary-shipped.json
//	go run ./cmd/adcvdeval -check                # recompute, fail on drift
//
// Inputs, from eval/adcvd: rulings.jsonl (the final scope rulings made on or
// after 1 January 2025, read from the Federal Register by rulings.py),
// labels.jsonl (each ruling's outcome and the HTS code of its product, from
// the gateway model, see prompts.json) and an orders snapshot. The check is
// deterministic. A ruling is scored when its outcome is within or outside
// and the model gave an HTS code; the check is asked about one part with that
// code and the country of the order. The check is right when it flags the
// ruling's own order (a match naming one of the ruling's case numbers) for a
// product found within the scope, and when it does not for a product found
// outside it.
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
	Title        string   `json:"title"`
	OrderCountry string   `json:"order_country"`
	RulingDate   string   `json:"ruling_date"`
}

type label struct {
	ID            string `json:"id"`
	Outcome       string `json:"outcome"`
	TurnsOnOrigin bool   `json:"turns_on_origin"`
	HTS           string `json:"hts"`
}

// Row is one scored ruling.
type Row struct {
	ID            string   `json:"id"`
	Cases         []string `json:"cases"`
	Outcome       string   `json:"outcome"`
	HTS           string   `json:"hts"`
	Origin        string   `json:"origin"`
	Status        string   `json:"status"`
	FlaggedOwn    bool     `json:"flagged_own_order"`
	FlaggedAny    bool     `json:"flagged_any_order"`
	MatchedCases  []string `json:"matched_cases,omitempty"`
	SnapshotGap   bool     `json:"order_not_in_snapshot"`
	TurnsOnOrigin bool     `json:"turns_on_origin"`
	Skip          string   `json:"skip,omitempty"`
}

// Rate is a count out of a total with its Wilson 95% interval.
type Rate struct {
	K, N int
	P    float64
	Lo   float64
	Hi   float64
}

// Confusion is the scoring of one flag against the outcomes.
type Confusion struct {
	TP, FP, FN, TN int
	Precision      Rate
	Recall         Rate
	Specificity    Rate
	Accuracy       Rate
}

// Summary is what the run reports.
type Summary struct {
	Snapshot          string         `json:"snapshot"`
	SnapshotOrders    int            `json:"snapshot_orders"`
	Rulings           int            `json:"rulings"`
	Scored            int            `json:"scored"`
	Within            int            `json:"within"`
	Outside           int            `json:"outside"`
	Skipped           map[string]int `json:"skipped"`
	OrderNotInSnap    int            `json:"within_on_orders_not_in_snapshot"`
	Own               Confusion      `json:"own_order"`
	Any               Confusion      `json:"any_order"`
	FlagEveryRuling   Confusion      `json:"baseline_flag_every_ruling"`
	OwnTurnsOnOrigin  Confusion      `json:"own_order_turns_on_origin"`
	OwnOtherwise      Confusion      `json:"own_order_other_rulings"`
	OwnInSnapshotOnly Confusion      `json:"own_order_orders_in_snapshot_only"`
}

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
	r.P, r.Lo, r.Hi = p, math.Max(0, (c-h)/d), math.Min(1, (c+h)/d)
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
	c.Accuracy = wilson(c.TP+c.TN, len(rows))
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

// score runs the check for every labelled ruling.
func score(t *adcvd.Table, rulings []ruling, labels map[string]label) ([]Row, Summary) {
	s := Summary{Snapshot: t.Snapshot(), SnapshotOrders: len(t.Orders()), Rulings: len(rulings), Skipped: map[string]int{}}
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
		row := Row{ID: r.ID, Cases: r.Cases, Outcome: l.Outcome, HTS: l.HTS, Origin: r.OrderCountry, TurnsOnOrigin: l.TurnsOnOrigin, SnapshotGap: !hasCase(t, r.Cases)}
		res := t.Check(adcvd.Input{Parts: []adcvd.Part{{Name: r.ID, HTS: l.HTS, Origin: r.OrderCountry}}})
		p := res.Parts[0]
		row.Status = p.Status
		if p.Status == adcvd.StatusNotChecked {
			s.Skipped["not_checked_"+p.Reason]++
			continue
		}
		row.FlaggedAny = p.Status == adcvd.StatusPossible
		for _, m := range p.Matches {
			for _, c := range m.Cases {
				for _, rc := range r.Cases {
					if c == rc {
						row.FlaggedOwn = true
						row.MatchedCases = append(row.MatchedCases, c)
					}
				}
			}
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
	own := func(r Row) bool { return r.FlaggedOwn }
	s.Own = confusion(rows, own)
	s.Any = confusion(rows, func(r Row) bool { return r.FlaggedAny })
	s.FlagEveryRuling = confusion(rows, func(Row) bool { return true })
	var turns, other, inSnap []Row
	for _, r := range rows {
		if r.TurnsOnOrigin {
			turns = append(turns, r)
		} else {
			other = append(other, r)
		}
		if !r.SnapshotGap {
			inSnap = append(inSnap, r)
		}
	}
	s.OwnTurnsOnOrigin, s.OwnOtherwise, s.OwnInSnapshotOnly = confusion(turns, own), confusion(other, own), confusion(inSnap, own)
	return rows, s
}

func writeJSONL(path string, rows []Row) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	for _, r := range rows {
		b, _ := json.Marshal(r)
		fmt.Fprintln(f, string(b))
	}
	return nil
}

func main() {
	dir := flag.String("dir", "eval/adcvd", "directory with rulings.jsonl and labels.jsonl")
	orders := flag.String("orders", "eval/adcvd/orders-2024-12-31.json", "orders snapshot")
	out := flag.String("out", "eval/adcvd/results.jsonl", "per-ruling results")
	sum := flag.String("summary", "eval/adcvd/summary.json", "summary")
	check := flag.Bool("check", false, "recompute and fail if the files on disk differ")
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
	rows, s := score(t, rulings, labels)
	b, _ := json.MarshalIndent(s, "", " ")
	b = append(b, '\n')
	if *check {
		have, err := os.ReadFile(*sum)
		must(err)
		if string(have) != string(b) {
			fmt.Fprintln(os.Stderr, "summary differs from a recomputation")
			os.Exit(1)
		}
		fmt.Println("adcvdeval: summary reproduced,", s.Scored, "rulings scored")
		return
	}
	must(writeJSONL(*out, rows))
	must(os.WriteFile(*sum, b, 0o644))
	fmt.Println(string(b))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "adcvdeval:", err)
		os.Exit(1)
	}
}
