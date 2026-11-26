// Command marketeval scores the rules-only market checks against decisions
// with a known outcome. No model call.
//
//	go run ./cmd/marketeval canada [-check]
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/euorigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ukorigin"
)

const evalDir = "../eval"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: marketeval canada|euuk [-check]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "canada":
		err = cmdCanada(os.Args[2:])
	case "euuk":
		err = cmdEUUK(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "marketeval:", err)
		os.Exit(1)
	}
}

type inputRow struct {
	Ruling string            `json:"ruling"`
	Split  string            `json:"split"`
	Source string            `json:"source"`
	Input  tariffshift.Input `json:"input"`
}

type goldRow struct {
	Ruling string `json:"ruling"`
	Gold   string `json:"gold"`
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
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

type caseOut struct {
	Ruling string `json:"ruling"`
	Split  string `json:"split"`
	Gold   string `json:"gold"`
	US     string `json:"us_status"`
	USOrig string `json:"us_origin,omitempty"`
	CA     string `json:"ca_status"`
	CAOrig string `json:"ca_origin,omitempty"`
	CARule string `json:"ca_rule,omitempty"`
}

type tally struct {
	N          int `json:"n"`
	Decided    int `json:"decided"`
	Agree      int `json:"agree"`
	Disagree   int `json:"disagree"`
	Undecided  int `json:"undetermined"`
	OutOfScope int `json:"out_of_scope"`
}

func decided(r tariffshift.Result) bool { return r.Status == "decided" }

func cmdCanada(args []string) error {
	fs := flag.NewFlagSet("canada", flag.ExitOnError)
	check := fs.Bool("check", false, "fail if the committed summary differs")
	_ = fs.Parse(args)
	ca, err := tariffshift.CanadaRules()
	if err != nil {
		return err
	}
	us, err := tariffshift.Default()
	if err != nil {
		return err
	}
	summary := map[string]any{
		"note": "Canada's SOR/94-23 Schedule III table and the U.S. 19 CFR 102.20 table, each applied to the same inputs read from CBP Part 102 rulings (eval/tariffshift/inputs-*-full_text.jsonl, extracted once per D360/D361). Gold is CBP's holding. The rulings decide U.S. marking of goods processed in Canada or Mexico; Canada's table is applied to the same facts as a rule-text test, not as a Canadian ruling. No model call. Recompute: cd backend && go run ./cmd/marketeval canada -check",
	}
	var cases []caseOut
	totals := map[string]*tally{"ca": {}, "us": {}}
	for _, split := range []string{"dev", "final"} {
		ins, err := readJSONL[inputRow](filepath.Join(evalDir, "tariffshift", "inputs-"+split+"-full_text.jsonl"))
		if err != nil {
			return err
		}
		golds, err := readJSONL[goldRow](filepath.Join(evalDir, "tariffshift", "results-"+split+"-full_text.jsonl"))
		if err != nil {
			return err
		}
		gold := map[string]string{}
		for _, g := range golds {
			gold[g.Ruling] = g.Gold
		}
		for _, in := range ins {
			g := gold[in.Ruling]
			rc, ru := ca.Evaluate(in.Input), us.Evaluate(in.Input)
			c := caseOut{Ruling: in.Ruling, Split: split, Gold: g, US: ru.Status, USOrig: ru.Origin, CA: rc.Status, CAOrig: rc.Origin}
			if rc.Rule != nil {
				c.CARule = rc.Rule.ID
			}
			cases = append(cases, c)
			for k, r := range map[string]tariffshift.Result{"ca": rc, "us": ru} {
				t := totals[k]
				t.N++
				switch {
				case decided(r) && strings.EqualFold(r.Origin, g):
					t.Decided++
					t.Agree++
				case decided(r):
					t.Decided++
					t.Disagree++
				case r.Status == "out_of_scope":
					t.OutOfScope++
				default:
					t.Undecided++
				}
			}
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Ruling < cases[j].Ruling })
	statuses := map[string]int{}
	for _, c := range cases {
		statuses["ca:"+c.CA]++
	}
	summary["canada"], summary["us_102_20"], summary["ca_statuses"], summary["cases"] = totals["ca"], totals["us"], statuses, cases
	b, err := json.MarshalIndent(summary, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	out := filepath.Join(evalDir, "markets", "canada-cbp-part102.json")
	if *check {
		old, err := os.ReadFile(out)
		if err != nil || !bytes.Equal(old, b) {
			return errors.Join(errors.New(out+" is stale: run go run ./cmd/marketeval canada"), err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	fmt.Printf("canada %+v\nus     %+v\nstatuses %v\n", *totals["ca"], *totals["us"], statuses)
	return os.WriteFile(out, b, 0o644)
}

type euukCase struct {
	ID       string         `json:"id"`
	Market   string         `json:"market"`
	Kind     string         `json:"source_kind"`
	Rules    string         `json:"rules_in_force"`
	Input    nporigin.Input `json:"input"`
	Expected string         `json:"expected_origin"`
	Conf     string         `json:"confidence"`
}

type euukOut struct {
	ID       string `json:"id"`
	Market   string `json:"market"`
	Rules    string `json:"rules_in_force"`
	Conf     string `json:"confidence"`
	Expected string `json:"expected"`
	Status   string `json:"status"`
	Origin   string `json:"origin,omitempty"`
	Reason   string `json:"reason"`
}

// writeOrCheck writes v as indented JSON to out, or with check compares.
func writeOrCheck(out string, v any, check bool, cmd string) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if check {
		old, err := os.ReadFile(out)
		if err != nil || !bytes.Equal(old, b) {
			return errors.Join(errors.New(out+" is stale: run go run ./cmd/marketeval "+cmd), err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o644)
}

func cmdEUUK(args []string) error {
	fs := flag.NewFlagSet("euuk", flag.ExitOnError)
	check := fs.Bool("check", false, "fail if the committed summary differs")
	_ = fs.Parse(args)
	eu, err := euorigin.Rules()
	if err != nil {
		return err
	}
	uk, err := ukorigin.Rules()
	if err != nil {
		return err
	}
	cases, err := readJSONL[euukCase](filepath.Join(evalDir, "markets", "eu-uk", "cases.jsonl"))
	if err != nil {
		return err
	}
	var outs []euukOut
	totals := map[string]*tally{}
	for _, c := range cases {
		r := eu
		if c.Market == "uk" {
			r = uk
		}
		res := r.Evaluate(c.Input)
		o := euukOut{ID: c.ID, Market: c.Market, Rules: c.Rules, Conf: c.Conf, Expected: c.Expected, Status: res.Status, Origin: res.Origin, Reason: res.Reason}
		outs = append(outs, o)
		for _, k := range []string{"all", c.Market, c.Rules} {
			t := totals[k]
			if t == nil {
				t = &tally{}
				totals[k] = t
			}
			t.N++
			switch {
			case res.Status == nporigin.Decided && strings.EqualFold(res.Origin, c.Expected):
				t.Decided++
				t.Agree++
			case res.Status == nporigin.Decided:
				t.Decided++
				t.Disagree++
			default:
				t.Undecided++
			}
		}
	}
	for k, t := range totals {
		fmt.Printf("%-11s %+v\n", k, *t)
	}
	summary := map[string]any{
		"note":   "EU and UK list-rule checks on hand-built cases from court, tribunal and Commission guidance texts with a stated origin (eval/markets/eu-uk/cases.jsonl). pre2016 cases were decided under the old Annex 11 / Art. 24 rules, not the Annex 22-01 rules the check applies. undetermined counts answers that need a person (no list rule, or a missing fact). No model call. Recompute: cd backend && go run ./cmd/marketeval euuk -check",
		"totals": totals,
		"cases":  outs,
	}
	return writeOrCheck(filepath.Join(evalDir, "markets", "eu-uk-cases.json"), summary, *check, "euuk")
}
