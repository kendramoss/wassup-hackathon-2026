package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// cmdRulesCA rebuilds the Canadian marking-rule table (SOR/94-23 Schedule
// III, D720) from the Justice Laws XML and prints its coverage and every
// alternative it could not encode. The XML is not committed; the table
// keeps the text of every row, and the package tests re-parse it.
func cmdRulesCA(args []string) error {
	fl := flag.NewFlagSet("rules-ca", flag.ExitOnError)
	src := fl.String("xml", "", "Justice Laws XML of SOR/94-23 (https://laws-lois.justice.gc.ca/eng/XML/SOR-94-23.xml)")
	retrieved := fl.String("retrieved", "", "date the XML was downloaded (YYYY-MM-DD)")
	out := fl.String("out", "internal/tariffshift/rules_ca.json", "output file")
	_ = fl.Parse(args)
	if *src == "" || *retrieved == "" {
		return fmt.Errorf("-xml and -retrieved are required")
	}
	xml, err := os.ReadFile(*src)
	if err != nil {
		return err
	}
	rules, err := tariffshift.BuildCanadaRules(xml)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(xml)
	var chapters []string
	for _, r := range rules {
		if !slices.Contains(chapters, r.Paragraph) {
			chapters = append(chapters, r.Paragraph)
		}
	}
	t := tariffshift.Table{
		Regime:  tariffshift.Canada.ID,
		Version: tariffshift.CanadaTableVersion,
		Source: tariffshift.TableSource{
			Title:     "SOR/94-23, Determination of Country of Origin for the Purpose of Marking Goods (CUSMA Countries) Regulations, Schedule III (Tariff Shift Rules)",
			URL:       "https://laws-lois.justice.gc.ca/eng/XML/SOR-94-23.xml",
			AsOf:      "2020-07-01",
			Retrieved: *retrieved,
			Note:      "Rule text from the Justice Laws consolidation (last amended 2020-07-01, consolidated to 2022-11-17). Two changes of form only: headings are written 8473 for 84.73, and code ranges use a hyphen for the en dash. Alternatives parsed by tariffshift.ParseRule; an alternative with encoded=false is never decided by code. Not the official version.",
			SHA256:    hex.EncodeToString(sum[:]),
			Licence:   "Reproduction of Federal Law Order (SI/97-5, https://laws-lois.justice.gc.ca/eng/regulations/SI-97-5/FullText.html, read 2026-11-22): anyone may, without charge or request for permission, reproduce enactments and consolidations of enactments of the Government of Canada, provided due diligence is exercised in ensuring the accuracy of the materials reproduced and the reproduction is not represented as an official version.",
		},
		Chapters: chapters,
		Rules:    rules,
	}
	if err := writeRules(*out, t); err != nil {
		return err
	}
	return printCoverage(t)
}

// printCoverage prints the table's coverage and each alternative not
// encoded, with the reason.
func printCoverage(t tariffshift.Table) error {
	c := tariffshift.CoverageOf(t)
	fmt.Printf("rows %d (fully encoded %d, partly %d, none %d); alternatives %d, encoded %d (%.1f%%)\n",
		c.Rows, c.RowsFull, c.RowsPartial, c.RowsNone, c.Alternatives, c.Encoded, 100*float64(c.Encoded)/float64(c.Alternatives))
	for _, r := range t.Rules {
		for i, a := range r.Alternatives {
			if !a.Encoded {
				fmt.Printf("%s alt %d: %s | %s\n", r.ID, i+1, a.Reason, a.Text)
			}
		}
	}
	return nil
}
