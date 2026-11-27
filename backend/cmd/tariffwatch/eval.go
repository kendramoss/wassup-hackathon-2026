package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

// The D941 accuracy test: which China tariff lines a newer HTS revision
// affects, and by how many points the additional duty changes, against the
// Federal Register notices (eval/tariffwatch). No model, no network.

const (
	snapshotDay = "2026-10-20"
	entryDay1   = "2026-11-27"
	entryDay2   = "2027-01-10"
)

// Confusion is one entry day's score of "affected".
type Confusion struct {
	TP, FP, FN, TN int
	Precision      float64  `json:"precision"`
	PrecisionLo    float64  `json:"precision_wilson_lo"`
	PrecisionHi    float64  `json:"precision_wilson_hi"`
	Recall         float64  `json:"recall"`
	RecallLo       float64  `json:"recall_wilson_lo"`
	RecallHi       float64  `json:"recall_wilson_hi"`
	FalsePositives []string `json:"false_positives"`
	FalseNegatives []string `json:"false_negatives"`
}

func score(pred, truth map[string]bool, universe []string) Confusion {
	var c Confusion
	for _, h := range universe {
		switch {
		case pred[h] && truth[h]:
			c.TP++
		case pred[h]:
			c.FP++
			c.FalsePositives = append(c.FalsePositives, h)
		case truth[h]:
			c.FN++
			c.FalseNegatives = append(c.FalseNegatives, h)
		default:
			c.TN++
		}
	}
	c.Precision, c.PrecisionLo, c.PrecisionHi = evalkit.Wilson(c.TP, c.TP+c.FP)
	c.Recall, c.RecallLo, c.RecallHi = evalkit.Wilson(c.TP, c.TP+c.FN)
	return c
}

// RateScore is the exact match of the predicted change in the additional
// duty rate with the documented one.
type RateScore struct {
	MaritimeExact int `json:"maritime_exact"`
	MaritimeN     int `json:"maritime_n"`
	AllExact      int `json:"all_exact"`
	AllN          int `json:"all_n"`
}

// Summary is the committed result (eval/tariffwatch/summary.json).
type Summary struct {
	Decision     string `json:"decision"`
	BaseRevision string `json:"base_revision"`
	DevRevision  string `json:"dev_revision"`
	HeldRevision string `json:"held_out_revision"`
	SnapshotDay  string `json:"snapshot_day"`
	Universe     int    `json:"universe_subheadings"`
	Truth        struct {
		ExclusionNumbers       int     `json:"exclusion_numbers"`
		ExclusionSubheadings   int     `json:"exclusion_subheadings"`
		MaritimeSubheadings    int     `json:"maritime_subheadings"`
		MaritimeRatePoints     float64 `json:"maritime_rate_change"`
		ExclusionsInData       int     `json:"exclusion_subheadings_in_the_data"`
		ExclusionsNotInTheData int     `json:"exclusion_subheadings_not_in_the_data"`
	} `json:"truth"`
	Dev struct {
		AffectedD1 int `json:"affected_entry_day_1"`
		AffectedD2 int `json:"affected_entry_day_2"`
	} `json:"development_21_to_22"`
	Held struct {
		D1   Confusion `json:"entry_day_1"`
		D2   Confusion `json:"entry_day_2"`
		Rate RateScore `json:"rate_change"`
	} `json:"held_out_22_to_23"`
	EntryDay1 string `json:"entry_day_1_date"`
	EntryDay2 string `json:"entry_day_2_date"`
}

func readFile(dir, name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	return string(b), err
}

// runEval measures the loader against the notices. evalDir holds the notice
// texts; excerptDir the Chapter 99 heading excerpts of Revisions 21, 22, 23.
func runEval(evalDir, excerptDir string) (Summary, error) {
	var s Summary
	s.Decision, s.SnapshotDay, s.EntryDay1, s.EntryDay2 = "D941", snapshotDay, entryDay1, entryDay2
	var readings [][]string
	for _, sc := range []string{"3", "4", "6"} {
		ocr, err := readFile(evalDir, "fr-89FR46948-annexC.ocr-scale"+sc+".txt")
		if err != nil {
			return s, err
		}
		readings = append(readings, exclusionNumbers(ocr))
	}
	solar, err := readFile(evalDir, "fr-89FR76581-annexB.txt")
	if err != nil {
		return s, err
	}
	mar, err := readFile(evalDir, "fr-90FR48320-annexVA.txt")
	if err != nil {
		return s, err
	}
	var rev [3]string
	for i, r := range []string{"21", "22", "23"} {
		if rev[i], err = readFile(excerptDir, "ch99_rev"+r+"_excerpt.txt"); err != nil {
			return s, err
		}
	}

	nums := append(votedNumbers(readings...), solarNumbers(solar)...)
	E := toSet(subheadings(nums))
	mLines, mRate := maritimeLines(mar)
	M := toSet(subheadings(mLines))
	s.Truth.ExclusionNumbers, s.Truth.ExclusionSubheadings = len(nums), len(E)
	s.Truth.MaritimeSubheadings, s.Truth.MaritimeRatePoints = len(M), mRate

	base, err := tariffwatch.Embedded()
	if err != nil {
		return s, err
	}
	s.BaseRevision = base.Revision
	d22, _, err := tariffwatch.Revise(base, rev[0], rev[1])
	if err != nil {
		return s, err
	}
	d23, _, err := tariffwatch.Revise(d22, rev[1], rev[2])
	if err != nil {
		return s, err
	}
	s.DevRevision, s.HeldRevision = d22.Revision, d23.Revision

	// The universe: every subheading the Revision 21 data gives a China
	// additional duty or an exclusion, and the maritime lines.
	u := map[string]bool{}
	for _, l := range base.Additional.Lists {
		for _, h := range l.Subheadings {
			u[duty.Normalize(h)] = true
		}
	}
	inData := map[string]bool{}
	for _, e := range base.Additional.Exclusions {
		u[duty.Normalize(e.HTS)] = true
		inData[duty.Normalize(e.HTS)] = true
	}
	for h := range M {
		u[h] = true
	}
	universe := sortedKeys(u)
	s.Universe = len(universe)
	for h := range E {
		if inData[h] {
			s.Truth.ExclusionsInData++
		} else {
			s.Truth.ExclusionsNotInTheData++
		}
	}

	evBase, err := tariffwatch.NewEvaluator(base)
	if err != nil {
		return s, err
	}
	evDev, _ := tariffwatch.NewEvaluator(d22)
	evHeld, _ := tariffwatch.NewEvaluator(d23)
	snap, _ := time.Parse(time.DateOnly, snapshotDay)
	days := [2]time.Time{}
	days[0], _ = time.Parse(time.DateOnly, entryDay1)
	days[1], _ = time.Parse(time.DateOnly, entryDay2)
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)

	type verdict struct {
		affected bool
		change   float64
	}
	check := func(ev *tariffwatch.Evaluator, day time.Time) (map[string]verdict, error) {
		out := make(map[string]verdict, len(universe))
		for _, h := range universe {
			ob, err := evBase.Evaluate(h, "China", nil, snap)
			if err != nil {
				return nil, err
			}
			b := tariffwatch.Basis{HTS: h, HTSRevision: base.Revision, PricedAt: snap, AsOf: snapshotDay, AnnualImportUSD: 1_000_000,
				Section232: tariffwatch.Section232{}, Origins: []tariffwatch.OriginBasis{ob}}
			res, err := ev.Watch(tariffwatch.CaseInput{Basis: b, BaseOrigin: "China"}, day, now)
			if err != nil {
				return nil, err
			}
			v := verdict{affected: res.Affected}
			for _, r := range res.Rates {
				v.change += r.Delta
			}
			out[h] = verdict{affected: v.affected, change: math.Round(v.change*1e4) / 1e4}
		}
		return out, nil
	}
	for i, day := range days {
		v, err := check(evDev, day)
		if err != nil {
			return s, err
		}
		n := 0
		for _, x := range v {
			if x.affected {
				n++
			}
		}
		if i == 0 {
			s.Dev.AffectedD1 = n
		} else {
			s.Dev.AffectedD2 = n
		}
	}
	var held [2]map[string]verdict
	for i, day := range days {
		if held[i], err = check(evHeld, day); err != nil {
			return s, err
		}
	}
	pred := func(v map[string]verdict) map[string]bool {
		m := map[string]bool{}
		for h, x := range v {
			m[h] = x.affected
		}
		return m
	}
	s.Held.D1 = score(pred(held[0]), E, universe)
	truth2 := map[string]bool{}
	for h := range E {
		truth2[h] = true
	}
	for h := range M {
		truth2[h] = true
	}
	s.Held.D2 = score(pred(held[1]), truth2, universe)
	// The documented change in the additional duty: +mRate for the maritime
	// lines on the second day, nothing anywhere else.
	for i := range held {
		for _, h := range universe {
			want := 0.0
			if i == 1 && M[h] {
				want = mRate
			}
			exact := math.Abs(held[i][h].change-want) < 1e-9
			s.Held.Rate.AllN++
			if exact {
				s.Held.Rate.AllExact++
			}
			if i == 1 && M[h] {
				s.Held.Rate.MaritimeN++
				if exact {
					s.Held.Rate.MaritimeExact++
				}
			}
		}
	}
	return s, nil
}

func toSet(xs []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func evalCmd(args []string, out io.Writer) error {
	fl := flag.NewFlagSet("eval", flag.ContinueOnError)
	evalDir := fl.String("eval-dir", "../eval/tariffwatch", "the notice texts")
	excerpts := fl.String("excerpts", "internal/tariffwatch/testdata", "the Chapter 99 heading excerpts")
	write := fl.Bool("write", false, "write summary.json into -eval-dir")
	if err := fl.Parse(args); err != nil {
		return err
	}
	s, err := runEval(*evalDir, *excerpts)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", " ")
	b = append(b, '\n')
	if *write {
		p := filepath.Join(*evalDir, "summary.json")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "wrote", p)
		return nil
	}
	_, err = out.Write(b)
	return err
}
