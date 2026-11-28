// Command atigaeval checks frozen ATIGA conformance examples without model calls.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/atiga"
)

type expectation struct {
	Outcome      string            `json:"outcome,omitempty"`
	Applied      string            `json:"applied,omitempty"`
	Alternative  map[string]string `json:"alternative,omitempty"`
	Accumulation *bool             `json:"accumulation,omitempty"`
}

type evidenceCase struct {
	ID     string      `json:"id"`
	Kind   string      `json:"kind"`
	Source string      `json:"source"`
	Input  atiga.Input `json:"input"`
	Expect expectation `json:"expect"`
}

type totals struct {
	Cases  int `json:"cases"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

type caseResult struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	Source   string       `json:"source"`
	Expected expectation  `json:"expected"`
	Actual   atiga.Result `json:"actual"`
	Passed   bool         `json:"passed"`
	Failures []string     `json:"failures"`
}

type report struct {
	DatasetSHA256 string            `json:"dataset_sha256"`
	RuleVersion   string            `json:"rule_version"`
	Total         totals            `json:"total"`
	Groups        map[string]totals `json:"groups"`
	Results       []caseResult      `json:"results"`
}

func validOutcome(s string) bool {
	return s == atiga.Yes || s == atiga.No || s == atiga.Undetermined
}

func evaluate(data []byte) (report, error) {
	var doc struct {
		Sources map[string]struct {
			URL string `json:"url"`
		} `json:"sources"`
		Cases []evidenceCase `json:"cases"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return report{}, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return report{}, fmt.Errorf("expected one JSON document")
	}
	if len(doc.Cases) == 0 {
		return report{}, fmt.Errorf("no evaluation cases")
	}
	r := report{DatasetSHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Groups: map[string]totals{}}
	seen := map[string]bool{}
	for _, c := range doc.Cases {
		if c.ID == "" || seen[c.ID] {
			return report{}, fmt.Errorf("missing or duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Kind != "authority_example" && c.Kind != "synthetic_control" {
			return report{}, fmt.Errorf("%s: unknown evidence kind %q", c.ID, c.Kind)
		}
		if doc.Sources[c.Source].URL == "" {
			return report{}, fmt.Errorf("%s: missing source URL", c.ID)
		}
		e := c.Expect
		if (e.Outcome == "" && e.Applied == "" && len(e.Alternative) == 0 && e.Accumulation == nil) || (e.Outcome != "" && !validOutcome(e.Outcome)) {
			return report{}, fmt.Errorf("%s: missing or invalid expectation", c.ID)
		}
		for rule, want := range e.Alternative {
			if rule == "" || !validOutcome(want) {
				return report{}, fmt.Errorf("%s: invalid alternative expectation", c.ID)
			}
		}
		a, err := atiga.Check(c.Input)
		if err != nil {
			return report{}, fmt.Errorf("%s: %w", c.ID, err)
		}
		row := caseResult{ID: c.ID, Kind: c.Kind, Source: c.Source, Expected: e, Actual: a, Failures: []string{}}
		check := func(field string, got, want any) {
			if got != want {
				row.Failures = append(row.Failures, fmt.Sprintf("%s: got %v, want %v", field, got, want))
			}
		}
		if e.Outcome != "" {
			check("outcome", a.Outcome, e.Outcome)
		}
		if e.Applied != "" {
			check("applied", a.Applied, e.Applied)
		}
		// Sort before checking so reports remain byte-reproducible.
		for _, rule := range sortedKeys(e.Alternative) {
			got := ""
			for _, alt := range a.Alternatives {
				if alt.Rule == rule {
					got = alt.Outcome
				}
			}
			check("alternative "+rule, got, e.Alternative[rule])
		}
		if e.Accumulation != nil {
			check("accumulation", a.Accumulation, *e.Accumulation)
		}
		if a.Rule != nil {
			r.RuleVersion = a.Rule.Version
		}
		row.Passed = len(row.Failures) == 0
		g := r.Groups[c.Kind]
		g.Cases++
		r.Total.Cases++
		if row.Passed {
			g.Passed++
			r.Total.Passed++
		} else {
			g.Failed++
			r.Total.Failed++
		}
		r.Groups[c.Kind] = g
		r.Results = append(r.Results, row)
	}
	return r, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func main() {
	cases := flag.String("cases", "../eval/atiga/evaluation-cases.json", "frozen cases JSON")
	flag.Parse()
	data, err := os.ReadFile(*cases)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	r, err := evaluate(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if r.Total.Failed != 0 {
		os.Exit(1)
	}
}
