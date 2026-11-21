// Command tariffshift builds and evaluates the 19 CFR 102.20 rule layer.
//
//	go run ./cmd/tariffshift rules -ecfr part102.xml   # rebuild internal/tariffshift/rules.json
//	go run ./cmd/tariffshift rules -reparse            # re-parse its committed text after a parser change
//	go run ./cmd/tariffshift extract -split dev -source facts   # read rule inputs with a model (gateway; D361)
//	go run ./cmd/tariffshift eval -check                # recompute the offline evaluation (no model calls)
//	go run ./cmd/tariffshift parts-gold|parts-suggest|parts-eval   # per-part HTS suggestions (D380, D381; parts.go)
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
		fmt.Fprintln(os.Stderr, "usage: tariffshift rules|extract|eval|parts-gold|parts-suggest|parts-eval [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "rules":
		err = cmdRules(os.Args[2:])
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
	fmt.Printf("%d rules in chapters %s; %d of %d alternatives encoded\n", len(t.Rules), strings.Join(tariffshift.ScopeChapters, ","), enc, alts)
	return nil
}
