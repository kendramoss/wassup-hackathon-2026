package main

// The recorded example on the site's /examples/rule-check page: one
// development ruling, the per-part codes the suggester returned in the
// committed D382 run, and what the deterministic rule check decides from
// them. Nothing is called; the file is rebuilt from committed rows:
//
//	go run ./cmd/tariffshift parts-example [-check]   (in make reproduce)

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// exampleRuling is a development ruling (never held out) that the rule check
// leaves open with the codes CBP's text states and decides with the
// suggested part codes and CBP's code for the good (D382).
const (
	exampleRuling = "N341003"
	exampleRun    = "suggest-dev-v1.jsonl"
	exampleOut    = "../frontend/src/lib/rule-check-example.json"
)

type examplePart struct {
	Name        string              `json:"name"`
	Origin      string              `json:"origin"`
	StatedHTS   string              `json:"stated_hts,omitempty"` // the code CBP's text states for it, if any
	Suggestions []hts.PartCandidate `json:"suggestions"`
}

type exampleFile struct {
	Note      string             `json:"note"`
	Ruling    string             `json:"ruling"`
	Date      string             `json:"date"`
	Split     string             `json:"split"`
	Run       string             `json:"run"`
	Model     string             `json:"model"`
	Prompt    string             `json:"prompt"`
	Product   string             `json:"product"`
	GoodHTS   string             `json:"good_hts"`
	Country   string             `json:"country"`
	Parts     []examplePart      `json:"parts"`
	Before    tariffshift.Result `json:"before"`
	After     tariffshift.Result `json:"after"`
	CBPOrigin string             `json:"cbp_origin"`
	Agrees    bool               `json:"agrees"`
}

func cmdPartsExample(args []string) error {
	fl := flag.NewFlagSet("parts-example", flag.ExitOnError)
	check := fl.Bool("check", false, "rebuild and compare with the committed file; write nothing")
	_ = fl.Parse(args)
	rules, err := tariffshift.Default()
	if err != nil {
		return err
	}
	labels, err := usmcaLabels()
	if err != nil {
		return err
	}
	sugg, err := readJSONL[SuggestRow](filepath.Join(partsDir, exampleRun))
	if err != nil {
		return err
	}
	inputs, err := readInputs(filepath.Join(evalDir, "inputs-dev-full_text.jsonl"))
	if err != nil {
		return err
	}
	var row *SuggestRow
	for i := range sugg {
		if sugg[i].Ruling == exampleRuling {
			row = &sugg[i]
		}
	}
	var in *InputRow
	for i := range inputs {
		if inputs[i].Ruling == exampleRuling {
			in = &inputs[i]
		}
	}
	lab, ok := labels[exampleRuling]
	if row == nil || in == nil || !ok || lab.Origin == nil {
		return fmt.Errorf("%s: no suggestion row, extraction or label", exampleRuling)
	}
	if row.Split != "dev" {
		return fmt.Errorf("%s: the example must be a development ruling", exampleRuling)
	}
	top := map[int][]hts.PartCandidate{}
	for _, p := range row.Parts {
		top[p.Index] = p.Candidates
	}
	ex := exampleFile{
		Note:   "Recorded example, rebuilt from committed files by `cd backend && go run ./cmd/tariffshift parts-example -check`. Suggestions: the committed D382 development run (no new model call). Rule check: the deterministic 19 CFR 102.20 layer on CBP's code for the good and the top suggestion for each part.",
		Ruling: exampleRuling, Date: lab.Date, Split: row.Split, Run: "eval/tariffshift/parts/" + exampleRun,
		Model: row.Model, Prompt: row.Prompt, Product: productName(lab.Subject),
		GoodHTS: in.Input.GoodHTS, Country: in.Input.Country, CBPOrigin: *lab.Origin,
		Before: rules.Evaluate(in.Input),
		After:  rules.Evaluate(withParts(in.Input, in.Input.GoodHTS, top)),
	}
	for i, m := range in.Input.Materials {
		ex.Parts = append(ex.Parts, examplePart{Name: m.Name, Origin: m.Origin, StatedHTS: m.HTS, Suggestions: top[i]})
	}
	ex.Agrees = ex.After.Status == tariffshift.Decided && *same(ex.After.Origin, ex.CBPOrigin)
	if ex.Before.Status == tariffshift.Decided || !ex.Agrees {
		return fmt.Errorf("%s no longer shows the check deciding only with the suggestions (before %s, after %s %s, CBP %s)",
			exampleRuling, ex.Before.Status, ex.After.Status, ex.After.Origin, ex.CBPOrigin)
	}
	b, _ := json.MarshalIndent(ex, "", " ")
	if err := writeOrCheck(exampleOut, append(b, '\n'), *check); err != nil {
		return err
	}
	fmt.Printf("parts-example %s: before %s, after %s %s, CBP %s\n", exampleRuling, ex.Before.Status, ex.After.Status, ex.After.Origin, ex.CBPOrigin)
	return nil
}
