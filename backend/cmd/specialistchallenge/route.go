package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

const routerModel = "gemini-3.8-flash"

var specialistRegimes = map[string]bool{"textile_102_21": true, "pharma_api": true, "usmca_part102": true}

// GradeRouting scores one routing row. The label is the specialist's own
// regime (appropriate cases) or substantial_transformation (negative controls).
func GradeRouting(c Case, r Row) Row {
	r.ID, r.Feature, r.Split = c.ID, c.Feature, c.Split
	if r.Error != "" {
		r.Verdict, r.Class = ErrorV, "error"
		return r
	}
	want := c.Expected.Origin
	if specialistRegimes[want] {
		if r.Origin == want {
			r.Verdict, r.Class = Match, "routed_correct"
		} else {
			r.Verdict, r.Class = Mismatch, "routed_wrong"
		}
		return r
	}
	switch {
	case r.Origin == want:
		r.Verdict, r.Class = Match, "kept_general"
	case specialistRegimes[r.Origin]:
		r.Verdict, r.Class = Mismatch, "false_specialist"
	default:
		r.Verdict, r.Class = Mismatch, "unclear_label"
	}
	return r
}

// RoutingSummary keeps the appropriate specialist cases and the negative
// controls as separate denominators.
type RoutingSummary struct {
	Counts
	Appropriate     map[string]Counts `json:"appropriate_by_group"`
	AppropriateAll  Counts            `json:"appropriate_all"`
	NegativeControl Counts            `json:"negative_controls"`
	FalseSpecialist int               `json:"negative_controls_sent_to_a_specialist"`
	Unclear         int               `json:"negative_controls_labelled_unclear_or_other"`
	ModelCalls      int               `json:"model_calls"`
}

func add(c *Counts, r Row) {
	c.Cases++
	switch r.Verdict {
	case Match:
		c.Matched++
	case Mismatch:
		c.Mismatched++
	default:
		c.Errors++
	}
}

// SummarizeRouting recomputes the routing counts from the rows.
func SummarizeRouting(cases []Case, rows []Row) (RoutingSummary, error) {
	s := RoutingSummary{Appropriate: map[string]Counts{}}
	if err := Complete(cases, rows); err != nil {
		return s, err
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, c := range cases {
		r := byID[c.ID]
		add(&s.Counts, r)
		if r.CalledModel {
			s.ModelCalls++
		}
		if c.ScenarioFamily == "negative-control" {
			add(&s.NegativeControl, r)
			switch r.Class {
			case "false_specialist":
				s.FalseSpecialist++
			case "unclear_label":
				s.Unclear++
			}
			continue
		}
		g := s.Appropriate[c.ScenarioFamily]
		add(&g, r)
		s.Appropriate[c.ScenarioFamily] = g
		add(&s.AppropriateAll, r)
	}
	return s, nil
}

func cmdRoute(args []string) error {
	fs := flag.NewFlagSet("route", flag.ExitOnError)
	split := fs.String("split", "", "dev or test")
	tag := fs.String("tag", "", "run name (required for dev)")
	freeze := fs.String("freeze", "", "full sha of the frozen commit (required for test)")
	capUSD := fs.Float64("cap", 0, "stop sending after this much reported spend (required)")
	workers := fs.Int("workers", 8, "parallel cases")
	_ = fs.Parse(args)
	if *split != "dev" && *split != "test" {
		return errors.New("-split must be dev or test")
	}
	if *capUSD <= 0 || (*split == "dev" && *tag == "") {
		return errors.New("-cap is required, and -tag for development")
	}
	dev, test, err := LoadRouting(dataRoot())
	if err != nil {
		return err
	}
	if p := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 100}); len(p) > 0 {
		return fmt.Errorf("routing datasets are not well formed:\n%s", strings.Join(p, "\n"))
	}
	cases := dev
	out := filepath.Join(dataRoot(), "results", "routing-dev-"+*tag+".jsonl")
	if *split == "test" {
		cases = test
		out = filepath.Join(dataRoot(), "results", "routing-test.jsonl")
		if err := FrozenGuard(git, *freeze, out); err != nil {
			return err
		}
	} else if _, err := os.Stat(out); err == nil {
		return fmt.Errorf("%s exists: choose a new -tag", filepath.Base(out))
	}
	model, err := newGateway()
	if err != nil {
		return err
	}
	rt, err := router.NewVersion(model, routerModel, 2000, router.PromptVersion)
	if err != nil {
		return err
	}
	var rows []Row
	var mu, spentMu sync.Mutex
	var spent float64
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for _, c := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func(c Case) {
			defer wg.Done()
			defer func() { <-sem }()
			spentMu.Lock()
			over := spent >= *capUSD
			spentMu.Unlock()
			var r Row
			if over {
				r = GradeRouting(c, Row{Error: "budget_cap_refused"})
			} else {
				tally := &llm.CostTally{}
				ctx, cancel := context.WithTimeout(llm.ContextWithCostTally(context.Background(), tally), 90*time.Second)
				start := time.Now()
				l, err := rt.Classify(ctx, c.Input.Facts)
				cancel()
				r = Row{CalledModel: true, LatencyMS: time.Since(start).Milliseconds(), CostUSD: tally.USD()}
				if err != nil {
					code, _ := logsafe.Classify(err)
					r.Error = string(code)
					if r.Error == "" {
						r.Error = "error"
					}
				} else {
					r.Provider, r.ModelID, r.PromptVersion = l.Provider, l.ModelID, l.PromptVersion
					r.Origin, r.Route, r.Outcome = l.Regime, l.Route, l.Rule
					r.InputTokens, r.OutputTokens = l.InputTokens, l.OutputTokens
				}
				r = GradeRouting(c, r)
				spentMu.Lock()
				spent += r.CostUSD
				spentMu.Unlock()
			}
			mu.Lock()
			rows = append(rows, r)
			appendPartial(out+".partial", r)
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	if err := Gate(rows); err != nil {
		return err
	}
	if err := writeRowsFile(out, rows); err != nil {
		return err
	}
	_ = os.Remove(out + ".partial")
	s, err := SummarizeRouting(cases, rows)
	if err != nil {
		return err
	}
	var lat []int64
	for _, r := range rows {
		if r.CalledModel {
			lat = append(lat, r.LatencyMS)
		}
	}
	fmt.Printf("routing %s %s: %d cases, %d matched, %d mismatched, %d errors; appropriate %d/%d, negative controls %d/%d (sent to a specialist %d); cost $%.4f; p50 %d ms p95 %d ms\n",
		*split, *tag, s.Cases, s.Matched, s.Mismatched, s.Errors, s.AppropriateAll.Matched, s.AppropriateAll.Cases, s.NegativeControl.Matched, s.NegativeControl.Cases,
		s.FalseSpecialist, spent, Percentile(lat, .5), Percentile(lat, .95))
	return appendAttempt(attempt{Time: time.Now().UTC().Format(time.RFC3339), Tag: *tag, Feature: RoutingFeature, Split: *split, File: filepath.Base(out),
		Cases: s.Cases, Matched: s.Matched, Errors: s.Errors, Cost: round6(spent), Calls: s.ModelCalls})
}
