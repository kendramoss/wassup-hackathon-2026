package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
)

// suitePhases plans the whole paid suite offline: the admission smoke check,
// the held_out run and the synthetic run.
func suitePhases(rates llmbudget.Rates, smokeMaxOut int, splits ...string) (map[string]llmbudget.Phase, error) {
	if len(splits) == 0 {
		splits = []string{"held_out", "synthetic"}
	}
	rules, err := jprules.Default()
	if err != nil {
		return nil, err
	}
	phases := map[string]llmbudget.Phase{"smoke": smokePhase(rates, smokeMaxOut)}
	for _, split := range splits {
		items, pool, _, err := loadCandidate(split)
		if err != nil {
			return nil, err
		}
		p, err := planItems(split, items, pool, rules)
		if err != nil {
			return nil, err
		}
		phases[split] = llmbudget.Phase{WorstUSD: p.worstCase(rates), Requests: p.Calls, RequestsSHA256: p.RequestsSHA256}
	}
	return phases, nil
}

// budgetInit creates the one ledger of the suite, offline. It plans smoke,
// held_out and synthetic together and refuses, creating nothing, when their
// worst cases do not fit the cap. Every later paid command opens this ledger
// and no other.
func budgetInit(args []string) error {
	fs := flag.NewFlagSet("budget-init", flag.ExitOnError)
	path := fs.String("budget-ledger", "", "new ledger file (must not exist; keep it outside the repository)")
	capUSD := fs.Float64("budget-usd", llmbudget.MaxCapUSD, "total USD cap for the whole suite")
	rin := fs.Float64("rate-in", 0, "verified input USD per million tokens")
	rout := fs.Float64("rate-out", 0, "verified output USD per million tokens")
	rsrc := fs.String("rates-source", "", "the quote or price page the rates were checked against")
	smokeOut := fs.Int("smoke-max-output", 64, "output cap of the smoke check, 1 to 256")
	splitList := fs.String("splits", "held_out,synthetic", "comma-separated splits the ledger plans: held_out, synthetic, dev_eval, dev_neg, fresh_1126 or fresh_1126_notwin")
	_ = fs.Parse(args)
	if fs.NArg() != 0 || *path == "" || *smokeOut < 1 || *smokeOut > 256 {
		return errors.New("budget-init needs -budget-ledger, rates and a 1 to 256 -smoke-max-output")
	}
	rates := llmbudget.Rates{InputPerMTok: *rin, OutputPerMTok: *rout, Source: *rsrc}
	splits := strings.Split(*splitList, ",")
	for _, sp := range splits {
		if sp != "held_out" && sp != "synthetic" && sp != "dev_eval" && sp != "dev_neg" && !isFresh(sp) && !isAnswered(sp) {
			return fmt.Errorf("budget-init: unknown split %q", sp)
		}
	}
	phases, err := suitePhases(rates, *smokeOut, splits...)
	if err != nil {
		return err
	}
	var total float64
	for _, name := range append([]string{"smoke"}, splits...) {
		p := phases[name]
		total += p.WorstUSD
		fmt.Printf("%-9s %4d requests, worst case $%.4f\n", name, p.Requests, p.WorstUSD)
	}
	fmt.Printf("suite worst case $%.4f against a cap of $%.2f\n", total, *capUSD)
	if _, err := llmbudget.Create(*path, *capUSD, llmbudget.Binding{Model: jpengine.DefaultModel, Rates: rates}, phases); err != nil {
		return err
	}
	fmt.Println("ledger created")
	return nil
}
