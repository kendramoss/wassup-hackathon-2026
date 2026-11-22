package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
)

// marketCmd runs the market detector (D680) over a set of texts and writes a
// summary. No model call.
//
//	go run ./cmd/eval market -set dev             # development rulings of every regime
//	go run ./cmd/eval market -set final           # first held-out split (377), once
//	go run ./cmd/eval market -set probes -probes ../eval/jurisdiction/probes-hand.jsonl
func marketCmd(args []string) error {
	fl := flag.NewFlagSet("market", flag.ExitOnError)
	set := fl.String("set", "dev", "dev | final | probes")
	probes := fl.String("probes", "", "probe file (JSONL id, want, text) for -set probes")
	factsDir := fl.String("facts", "data/facts", "ruling facts directory")
	splitsDir := fl.String("splits", "../eval/splits", "splits directory")
	resultsDir := fl.String("results", "../eval/results", "where the summary and RUNLOG go")
	tag := fl.String("tag", "d680", "run tag")
	explain := fl.Bool("explain", false, "with -write=false, print each non-U.S. cue in context")
	write := fl.Bool("write", true, "write the summary and a RUNLOG row (false: print flagged rows only, for tuning on dev)")
	_ = fl.Parse(args)

	type item struct{ ID, Want, Text string }
	var items []item
	switch *set {
	case "dev", "final":
		ids := map[string]bool{}
		lists := map[string]string{"splits_final.json": *set}
		if *set == "dev" {
			lists = map[string]string{"splits_final.json": "dev", "splits_textile.json": "dev", "splits_usmca.json": "dev", "splits_pharma.json": "dev"}
		}
		for file, key := range lists {
			var m map[string]json.RawMessage
			b, err := os.ReadFile(filepath.Join(*splitsDir, file))
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, &m); err != nil {
				return err
			}
			var l []string
			if err := json.Unmarshal(m[key], &l); err != nil {
				return fmt.Errorf("%s %s: %w", file, key, err)
			}
			for _, id := range l {
				ids[id] = true
			}
		}
		for id := range ids {
			b, err := os.ReadFile(filepath.Join(*factsDir, id+".txt"))
			if err != nil {
				return err
			}
			items = append(items, item{id, jurisdiction.US, string(b)})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	case "probes":
		f, err := os.Open(*probes)
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var p struct{ ID, Want, Text string }
			if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
				return err
			}
			items = append(items, item{p.ID, p.Want, p.Text})
		}
		if err := sc.Err(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("market: unknown -set %q", *set)
	}

	type row struct {
		ID       string `json:"id"`
		Want     string `json:"want"`
		Got      string `json:"got"`
		Source   string `json:"source"`
		Cue      string `json:"cue,omitempty"`
		Hint     string `json:"hint,omitempty"`
		Conflict bool   `json:"conflict,omitempty"`
		// Kept is true when the case cannot get a U.S. auto-answer from the
		// detector's reading: a non-U.S. market, a conflict, or a hint
		// (a hint reviews only when the router says unclear).
		Kept bool `json:"kept_from_us_answer"`
	}
	var rows []row
	texts := map[string]string{}
	for _, it := range items {
		texts[it.ID] = it.Text
	}
	var nonUS, exact, kept, keptStrict, controls, controlsClean int
	for _, it := range items {
		j := jurisdiction.Detect(it.Text)
		r := row{ID: it.ID, Want: it.Want, Got: j.Code, Source: j.Source, Cue: j.Cue, Hint: j.Hint, Conflict: j.Conflict,
			Kept: j.Code != jurisdiction.US || j.Conflict || j.Hint != ""}
		rows = append(rows, r)
		if it.Want == jurisdiction.US {
			controls++
			if j.Code == jurisdiction.US && !j.Conflict && j.Hint == "" {
				controlsClean++
			}
			continue
		}
		nonUS++
		if j.Code == it.Want {
			exact++
		}
		if r.Kept {
			kept++
		}
		if j.Code != jurisdiction.US || j.Conflict {
			keptStrict++
		}
	}
	if !*write {
		for _, r := range rows {
			if r.Want != r.Got || r.Conflict || r.Hint != "" {
				fmt.Printf("%s want=%s got=%s cue=%q hint=%s conflict=%v\n", r.ID, r.Want, r.Got, r.Cue, r.Hint, r.Conflict)
				if *explain {
					for _, m := range jurisdiction.Matches(texts[r.ID]) {
						if m.Market != jurisdiction.US {
							fmt.Printf("    %s strong=%v %q\n", m.Market, m.Strong, m.Snippet)
						}
					}
				}
			}
		}
		fmt.Printf("%d texts; U.S. %d (clean %d); non-U.S. %d: exact %d, kept %d (without router %d)\n", len(items), controls, controlsClean, nonUS, exact, kept, keptStrict)
		return nil
	}
	name := "market-detect-" + *set
	if *set == "probes" {
		name += "-" + strings.TrimSuffix(filepath.Base(*probes), ".jsonl")
	}
	sum := map[string]any{
		"run": name + "-" + *tag, "detector": jurisdiction.DetectorVersion, "time": time.Now().Format(time.RFC3339),
		"git_revision": revision(), "texts": len(items),
		"us_texts": controls, "us_texts_clean": controlsClean,
		"non_us_texts": nonUS, "exact_market": exact,
		"kept_from_us_answer": kept, "kept_without_router": keptStrict,
		"note": "kept_from_us_answer counts a hint, which reviews only when the router says unclear; kept_without_router counts only a non-U.S. market or a conflict",
		"rows": rows,
	}
	b, _ := json.MarshalIndent(sum, "", "  ")
	out := filepath.Join(*resultsDir, name+"-"+*tag+".json")
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	lf, err := os.OpenFile(filepath.Join(*resultsDir, "RUNLOG.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close()
	entry := map[string]any{"time": sum["time"], "run": sum["run"], "split": *set, "held_out": *set != "dev",
		"detector": jurisdiction.DetectorVersion, "cases": len(items), "us_texts": controls, "us_texts_clean": controlsClean,
		"non_us_texts": nonUS, "exact_market": exact, "kept_from_us_answer": kept, "kept_without_router": keptStrict,
		"cost_usd": 0, "git_revision": revision()}
	lb, _ := json.Marshal(entry)
	if _, err := lf.Write(append(lb, '\n')); err != nil {
		return err
	}
	fmt.Printf("%s: %d texts; U.S. %d (clean %d); non-U.S. %d: exact %d, kept %d (without router %d) -> %s\n",
		sum["run"], len(items), controls, controlsClean, nonUS, exact, kept, keptStrict, out)
	return nil
}
