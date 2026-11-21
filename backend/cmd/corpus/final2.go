package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
)

// readRaw loads one saved CROSS ruling.
func readRaw(rawDir, id string) (cross.Ruling, error) {
	var r cross.Ruling
	b, err := os.ReadFile(filepath.Join(rawDir, id+".json"))
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}

// labelCheck measures how well the Go labeller (corpus.LabelRuling: regime,
// multi-country flag and regex holding) reproduces labels.jsonl, the labels
// the first held-out split was built from. No model calls. Writes a JSON
// summary to -out (D150).
func labelCheck(args []string) error {
	fl := flag.NewFlagSet("label-check", flag.ExitOnError)
	labelsPath := fl.String("labels", "../eval/data/labels.jsonl", "reference labels")
	rawDir := fl.String("raw", "../data/corpus/raw", "CROSS JSON directory")
	out := fl.String("out", "../eval/data/final2_labeller_check.json", "summary output")
	_ = fl.Parse(args)
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	type cell struct{ Ref, Got string }
	conf := map[cell]int{}
	var n, multiAgree, noRaw int
	// Origin agreement on the rulings that define a split: regex-labelled,
	// substantial transformation, multi-country, in the reference.
	var stRegex, stOriginAgree, stBothOrigin int
	var stDisagree []string
	for _, ref := range labels {
		r, err := readRaw(*rawDir, ref.Ruling)
		if err != nil {
			noRaw++
			continue
		}
		_, meta := corpus.SplitFacts(r.Text)
		got := corpus.LabelRuling(r.RulingNumber, r.RulingDate, r.Collection, r.Subject, r.Text, meta)
		n++
		conf[cell{ref.Regime, got.Regime}]++
		if got.MultiCountry == ref.MultiCountry {
			multiAgree++
		}
		if ref.Regime == corpus.RegimeST && ref.MultiCountry && ref.Method == "regex" {
			stRegex++
			if got.Origin() == ref.Origin() {
				stOriginAgree++
				if ref.Origin() != "" {
					stBothOrigin++
				}
			} else if len(stDisagree) < 40 {
				stDisagree = append(stDisagree, fmt.Sprintf("%s: reference %q, labeller %q", ref.Ruling, ref.Origin(), got.Origin()))
			}
		}
	}
	var matrix []map[string]any
	for c, v := range conf {
		matrix = append(matrix, map[string]any{"reference": c.Ref, "labeller": c.Got, "n": v})
	}
	sort.Slice(matrix, func(i, j int) bool {
		a, b := matrix[i], matrix[j]
		if a["reference"] != b["reference"] {
			return a["reference"].(string) < b["reference"].(string)
		}
		return a["labeller"].(string) < b["labeller"].(string)
	})
	stTP, stRef, stGot := conf[cell{corpus.RegimeST, corpus.RegimeST}], 0, 0
	for c, v := range conf {
		if c.Ref == corpus.RegimeST {
			stRef += v
		}
		if c.Got == corpus.RegimeST {
			stGot += v
		}
	}
	sum := map[string]any{
		"reference": *labelsPath, "compared": n, "no_raw_text": noRaw,
		"regime_confusion":              matrix,
		"st_regime_precision":           ratio(stTP, stGot),
		"st_regime_recall":              ratio(stTP, stRef),
		"multi_country_agreement":       ratio(multiAgree, n),
		"st_regex_rows":                 stRegex,
		"st_regex_origin_agreement":     ratio(stOriginAgree, stRegex),
		"st_regex_origin_agree_count":   stOriginAgree,
		"st_regex_origin_agree_nonnull": stBothOrigin,
		"st_regex_origin_disagreements": stDisagree,
	}
	b, _ := json.MarshalIndent(sum, "", "  ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "label-check: %d rulings; ST regime precision %.3f recall %.3f; multi-country agreement %.3f; ST regex origin agreement %d/%d -> %s\n",
		n, ratio(stTP, stGot), ratio(stTP, stRef), ratio(multiAgree, n), stOriginAgree, stRegex, *out)
	return nil
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(int(float64(a)/float64(b)*10000+0.5)) / 10000
}

// final2Label labels the rulings the probe saved (dated on or after -since and
// not already in a labels file) with corpus.LabelRuling, writes their facts
// to -facts and the labels to -out. Manual corrections (-manual, a JSON object
// ruling -> {"primary_origin": "...", "note": "..."}) are applied last and
// marked method "manual" (D150).
func final2Label(args []string) error {
	fl := flag.NewFlagSet("final2-label", flag.ExitOnError)
	index := fl.String("index", "../eval/data/final2_probe.jsonl", "probe index")
	rawDir := fl.String("raw", "../data/corpus/raw", "CROSS JSON directory")
	since := fl.String("since", "2026-09-26", "first ruling date")
	existing := fl.String("existing", "../eval/data/labels.jsonl,../eval/data/labels_usmca.jsonl", "labels files whose rulings are skipped (comma-separated)")
	manualPath := fl.String("manual", "../eval/data/final2_manual.json", "manual label corrections")
	factsDir := fl.String("facts", "../data/corpus/facts", "facts directory to write")
	out := fl.String("out", "../eval/data/labels_final2.jsonl", "output labels")
	_ = fl.Parse(args)

	skip := map[string]bool{}
	for _, p := range strings.Split(*existing, ",") {
		ls, err := corpus.LoadLabels(p)
		if err != nil {
			return err
		}
		for _, l := range ls {
			skip[l.Ruling] = true
		}
	}
	manual := map[string]struct {
		PrimaryOrigin *string `json:"primary_origin"`
		Note          string  `json:"note"`
	}{}
	if b, err := os.ReadFile(*manualPath); err == nil {
		if err := json.Unmarshal(b, &manual); err != nil {
			return fmt.Errorf("%s: %w", *manualPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.Open(*index)
	if err != nil {
		return err
	}
	defer f.Close()
	ids := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var p probeRow
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			return err
		}
		if p.Saved && p.Date >= *since && !skip[p.Ruling] {
			ids[p.Ruling] = true
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	var rows []corpus.Label
	noFacts := 0
	for id := range ids {
		r, err := readRaw(*rawDir, id)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		facts, meta := corpus.SplitFacts(r.Text)
		if strings.TrimSpace(facts) == "" {
			noFacts++
			continue
		}
		if err := os.WriteFile(filepath.Join(*factsDir, id+".txt"), []byte(facts+"\n"), 0o644); err != nil {
			return err
		}
		l := corpus.LabelRuling(r.RulingNumber, r.RulingDate, r.Collection, r.Subject, r.Text, meta)
		if m, ok := manual[id]; ok {
			l.PrimaryOrigin, l.Method = m.PrimaryOrigin, "manual"
		}
		rows = append(rows, l)
	}
	for id := range manual {
		if !ids[id] {
			return fmt.Errorf("%s: manual correction for a ruling that is not a candidate", id)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].Ruling < rows[j].Ruling
	})
	w, err := os.Create(*out)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	for _, l := range rows {
		b, _ := json.Marshal(l)
		_, _ = bw.Write(append(b, '\n'))
	}
	if err := errors.Join(bw.Flush(), w.Close()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "final2-label: %d rulings labelled (%d manual), %d without a facts section -> %s\n", len(rows), len(manual), noFacts, *out)
	return nil
}
