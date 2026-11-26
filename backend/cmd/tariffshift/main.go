// Command tariffshift builds and evaluates the 19 CFR 102.20 rule layer.
//
//	go run ./cmd/tariffshift rules -ecfr part102.xml   # rebuild internal/tariffshift/rules.json
//	go run ./cmd/tariffshift rules -reparse            # re-parse its committed text after a parser change
//	go run ./cmd/tariffshift rules-ca -xml SOR-94-23.xml -retrieved 2026-11-22   # rebuild internal/tariffshift/rules_ca.json (Canada, D720; rules_ca.go)
//	go run ./cmd/tariffshift extract -split dev -source facts   # read rule inputs with a model (gateway; D361)
//	go run ./cmd/tariffshift eval -check                # recompute the offline evaluation (no model calls)
//	go run ./cmd/tariffshift parts-gold|parts-suggest|parts-eval   # per-part HTS suggestions (D380, D381; parts.go)
//	go run ./cmd/tariffshift parts-example -check       # the site's recorded rule-check example (example.go)
//	go run ./cmd/tariffshift plans-eval -check          # the rule check on each committed plan move (D440; plans.go)
//	go run ./cmd/tariffshift plans-example -check       # the site's recorded plan move check (plans_example.go)
//	go run ./cmd/tariffshift ground102-eval -check      # usmca-v1 against usmca-v2 on the dev Part 102 rulings (D550; ground102.go)
//	go run ./cmd/tariffshift p102ck-eval -check         # the D600 compact checklists on the dev Part 102 rulings and, if they pass, held out (p102ck.go)
//	go run ./cmd/tariffshift p102bar -check             # the usmca-v5 auto-accept bar rule (D610) and the Part 102 latency tail (D611) from committed rows (p102bar.go)
//
// The eCFR XML is the full text of 19 CFR Part 102 from
// https://www.ecfr.gov/api/versioner/v1/full/<date>/title-19.xml?part=102
// (public domain). It is not committed; rules.json keeps the verbatim text of
// every rule in scope, and the package tests re-parse it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tariffshift rules|rules-ca|rules-mx|extract|eval|parts-gold|parts-suggest|parts-eval|parts-example|plans-eval|plans-example [flags]")
		fmt.Fprintln(os.Stderr, "usage: tariffshift rules|rules-ca|rules-mx|extract|eval|parts-gold|parts-suggest|parts-eval|parts-example|plans-eval|plans102-eval|ground102-eval|p102err-single|p102err-moves|p102ck-eval|p102bar [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "rules":
		err = cmdRules(os.Args[2:])
	case "rules-ca":
		err = cmdRulesCA(os.Args[2:])
	case "rules-mx":
		err = cmdRulesMX(os.Args[2:])
	case "extract":
		err = cmdExtract(os.Args[2:])
	case "eval":
		err = cmdEval(os.Args[2:])
	case "parts-gold":
		err = cmdPartsGold(os.Args[2:])
	case "parts-suggest":
		err = cmdPartsSuggest(os.Args[2:])
	case "parts-eval":
		err = cmdPartsEval(os.Args[2:])
	case "parts-example":
		err = cmdPartsExample(os.Args[2:])
	case "plans-eval":
		err = cmdPlansEval(os.Args[2:])
	case "plans-example":
		err = cmdPlansExample(os.Args[2:])
	case "plans102-example":
		err = cmdPlans102Example(os.Args[2:])
	case "plans102-eval":
		err = cmdPlans102Eval(os.Args[2:])
	case "p102err-moves":
		err = cmdP102ErrMoves(os.Args[2:])
	case "p102err-single":
		err = cmdP102ErrSingle(os.Args[2:])
	case "ground102-eval":
		err = cmdGround102Eval(os.Args[2:])
	case "p102ck-eval":
		err = cmdP102ckEval(os.Args[2:])
	case "p102bar":
		err = cmdP102Bar(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tariffshift:", err)
		os.Exit(1)
	}
}

func cmdRules(args []string) error {
	fl := flag.NewFlagSet("rules", flag.ExitOnError)
	src := fl.String("ecfr", "", "eCFR XML of 19 CFR Part 102")
	asOf := fl.String("as-of", "", "the eCFR point-in-time date of the XML (YYYY-MM-DD)")
	retrieved := fl.String("retrieved", "", "date the XML was downloaded (YYYY-MM-DD)")
	out := fl.String("out", "internal/tariffshift/rules.json", "output file")
	reparse := fl.Bool("reparse", false, "parse the committed rule text in -out again with the current parser (no eCFR download)")
	_ = fl.Parse(args)
	var t tariffshift.Table
	if *reparse {
		r, err := tariffshift.Default()
		if err != nil {
			return err
		}
		if t, err = tariffshift.Reparse(r.Table()); err != nil {
			return err
		}
		return writeRules(*out, t)
	}
	if *src == "" || *asOf == "" || *retrieved == "" {
		return fmt.Errorf("-ecfr, -as-of and -retrieved are required (or -reparse)")
	}
	xml, err := os.ReadFile(*src)
	if err != nil {
		return err
	}
	rules, err := tariffshift.BuildRules(xml, tariffshift.ScopeChapters)
	if err != nil {
		return err
	}
	t = tariffshift.Table{
		Source: tariffshift.TableSource{
			Title:     "19 CFR 102.20, Specific rules by tariff classification",
			URL:       "https://www.ecfr.gov/api/versioner/v1/full/" + *asOf + "/title-19.xml?part=102",
			AsOf:      *asOf,
			Retrieved: *retrieved,
			Note:      "Verbatim rule text from the eCFR (public domain) for the chapters in scope; alternatives parsed by tariffshift.ParseRule. An alternative with encoded=false is never decided by code.",
		},
		Chapters: tariffshift.ScopeChapters,
		Rules:    rules,
	}
	return writeRules(*out, t)
}

func writeRules(out string, t tariffshift.Table) error {
	b, err := json.MarshalIndent(t, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	enc, alts := 0, 0
	for _, r := range t.Rules {
		for _, a := range r.Alternatives {
			alts++
			if a.Encoded {
				enc++
			}
		}
	}
	fmt.Printf("%d rules in chapters %s; %d of %d alternatives encoded\n", len(t.Rules), strings.Join(t.Chapters, ","), enc, alts)
	return nil
}
