package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
)

// The third held-out set (D957): New York rulings of 2014-2018, a period that
// no development, held-out or earlier-fresh step covered (development is
// Jul-Dec 2025, D952 is 2026, D956 is 2019-02 to 2022-10 and 2023-01 to
// 2025-06). Built with no model call and no rule output read.

var rulingNumber = regexp.MustCompile(`\b[NH][0-9]{6}\b`)

// mentionedRulings returns every ruling number written in the files under the
// given roots (code, tests, documents, earlier sets). A ruling named there is
// left out of the third set: its author may have read it.
func mentionedRulings(roots []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == ".next" {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".go", ".md", ".json", ".jsonl", ".ts", ".tsx", ".py", ".sh", ".yaml", ".txt":
			default:
				return nil
			}
			if filepath.Base(p) == "labels-textile.jsonl" || strings.HasPrefix(filepath.Base(p), "splits_textile_third") {
				return nil // this command's own output
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range rulingNumber.FindAll(b, -1) {
				out[string(m)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// periodOf names the period a ruling date belongs to: before the split date
// "a", on or after it "b".
func periodOf(date, splitDate string) string {
	if date < splitDate {
		return "a"
	}
	return "b"
}

// cmdBuildThird writes splits_textile_third.json, the facts of every ruling in
// it (data/facts_third/) and its labels (appended to labels-textile.jsonl as
// set "third").
//
//	go run ./cmd/textilerules build-third -raw DIR -from 2014-01-01 -to 2018-12-31 -split-date 2017-01-01 -frozen-at ISO
func cmdBuildThird(args []string) error {
	fl := flag.NewFlagSet("build-third", flag.ExitOnError)
	raw := fl.String("raw", "", "directory of raw CROSS JSON (not committed)")
	from := fl.String("from", "2014-01-01", "first date")
	to := fl.String("to", "2018-12-31", "last date")
	splitDate := fl.String("split-date", "2017-01-01", "the two periods meet here")
	frozen := fl.String("frozen-at", "", "freeze time (ISO 8601)")
	_ = fl.Parse(args)
	if *raw == "" || *frozen == "" {
		return fmt.Errorf("-raw and -frozen-at are required")
	}
	mentioned, err := mentionedRulings([]string{
		"internal", "cmd", "../docs", "../eval/splits", "../eval/textile102", "../frontend/src", "../frontend/scripts",
	})
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(*raw, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	left := map[string]string{}
	var labels []TextileLabel
	periods := map[string][]string{}
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".json")
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var r cross.Ruling
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		date := r.RulingDate[:min(10, len(r.RulingDate))]
		switch {
		case date < *from || date > *to:
			continue // outside the period: not a candidate, not listed
		case !strings.Contains(r.Text, "102.21"):
			continue
		case mentioned[id]:
			left[id] = "named in the rule code, tests, documents, frontend or an earlier set"
			continue
		}
		facts, meta := corpus.SplitFacts(r.Text)
		l := corpus.LabelHoldingTextile(r.RulingNumber, r.RulingDate, r.Collection, r.Subject, r.Text, meta)
		switch {
		case l.Regime != corpus.RegimePart102:
			left[id] = "regime " + l.Regime
		case l.Origin() == "":
			left[id] = "no single held origin"
		case strings.TrimSpace(facts) == "":
			left[id] = "no facts"
		default:
			if err := os.MkdirAll("data/facts_third", 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join("data/facts_third", id+".txt"), []byte(facts+"\n"), 0o644); err != nil {
				return err
			}
			labels = append(labels, TextileLabel{Label: l, Set: thirdSplit})
			p := periodOf(l.Date, *splitDate)
			periods[p] = append(periods[p], id)
		}
	}
	sort.SliceStable(labels, func(i, j int) bool {
		if labels[i].Date != labels[j].Date {
			return labels[i].Date < labels[j].Date
		}
		return labels[i].Ruling < labels[j].Ruling
	})
	ids := make([]string, len(labels))
	for i, l := range labels {
		ids[i] = l.Ruling
	}
	for _, p := range periods {
		sort.Strings(p)
	}
	reasons := map[string]int{}
	for _, why := range left {
		reasons[why]++
	}
	out := map[string]any{
		"source":           fmt.Sprintf("New York rulings dated %s to %s that cite 19 CFR 102.21, regime part102_tariff_shift, one held origin under the textile labeller, with a facts file; not named in the rule code, its tests, the documents, the frontend or an earlier set (D957)", *from, *to),
		"frozen_at":        *frozen,
		"third":            ids,
		"periods":          periods,
		"split_date":       *splitDate,
		"date_range":       []string{labels[0].Date, labels[len(labels)-1].Date},
		"left_out":         left,
		"left_out_reasons": reasons,
	}
	jb, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile("../eval/splits/splits_textile_third.json", append(jb, '\n'), 0o644); err != nil {
		return err
	}
	// Append the third set's labels to the file the five seen sets are in.
	old, err := os.ReadFile(textileLabelsFile)
	if err != nil {
		return err
	}
	var kept []byte
	for _, line := range strings.SplitAfter(string(old), "\n") {
		if line != "" && !strings.Contains(line, `"set":"`+thirdSplit+`"`) {
			kept = append(kept, line...)
		}
	}
	for _, l := range labels {
		b, _ := json.Marshal(l)
		kept = append(kept, append(b, '\n')...)
	}
	if err := os.WriteFile(textileLabelsFile, kept, 0o644); err != nil {
		return err
	}
	fmt.Printf("third: %d rulings (%d in period a, %d in period b), %v to %v; left out %v\n",
		len(ids), len(periods["a"]), len(periods["b"]), labels[0].Date, labels[len(labels)-1].Date, reasons)
	return nil
}
