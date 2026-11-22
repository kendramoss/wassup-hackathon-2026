package main

// The held origins behind revocation-alert relevance (D531): for every
// ruling in the supersession map, changed or changing, the single origin
// its label gives. Read from the labels the server ships
// (backend/data/labels.jsonl and labels_usmca.jsonl), never a held-out
// file. No model call:
//
//	go run ./cmd/corpus holdings [-check]   (in make reproduce)

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

const holdingsOut = "backend/internal/domain/ruling_holdings.json"

// recital matches a holding sentence that opens by recounting an earlier
// ruling: "In NY N331974, CBP found ...", "In HQ H012345, ...".
var recital = regexp.MustCompile(`^\s*In (?:NY|HQ) [A-Z]?\d+`)

var holdingsLabels = []string{"backend/data/labels.jsonl", "backend/data/labels_usmca.jsonl"}

func rulingHoldings(args []string) error {
	fl := flag.NewFlagSet("holdings", flag.ExitOnError)
	root := fl.String("root", "..", "repository root")
	check := fl.Bool("check", false, "fail when the committed file is stale")
	_ = fl.Parse(args)

	b, err := os.ReadFile(filepath.Join(*root, "backend", "internal", "domain", "supersession.json"))
	if err != nil {
		return err
	}
	var m domain.SupersessionMap
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	ids := map[string]bool{}
	changing := map[string]bool{}
	for _, e := range m.Edges {
		ids[e.Ruling], ids[e.By] = true, true
		changing[e.By] = true
	}
	for _, p := range m.Proposed {
		ids[p.Ruling], ids[p.By] = true, true
		changing[p.By] = true
	}

	held := map[string]string{}
	var recitals []string
	seen := map[string]bool{}
	for _, f := range holdingsLabels {
		ls, err := corpus.LoadLabels(filepath.Join(*root, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		for _, l := range ls {
			if !ids[l.Ruling] {
				continue
			}
			o := domain.NormalizeCountry(l.Origin()) // "" when the label gives no primary origin
			if strings.Contains(o, "/") {
				o = "" // several origins: not one holding to compare
			}
			if changing[l.Ruling] && len(l.Evidence) > 0 && recital.MatchString(l.Evidence[0]) {
				// The label was read from the letter's account of the
				// earlier ruling ("In NY N331974, CBP found ..."), not
				// from its own holding (H325607): not known.
				o = ""
				recitals = append(recitals, l.Ruling)
			}
			if seen[l.Ruling] && held[l.Ruling] != o {
				o = "" // two labels disagree: not known
			}
			held[l.Ruling], seen[l.Ruling] = o, true
		}
	}
	known := 0
	for _, o := range held {
		if o != "" {
			known++
		}
	}
	out := domain.RulingHoldings{
		Note: fmt.Sprintf("Held origins for the rulings in supersession.json (D531), from the shipped labels (%s and %s); \"\" when the label gives no single origin, or when a changing ruling's label was read from its account of the earlier ruling (%s). %d of %d rulings in the map have a label, %d with a single origin. Rebuilt by `cd backend && go run ./cmd/corpus holdings`.",
			holdingsLabels[0], holdingsLabels[1], strings.Join(recitals, ", "), len(held), len(ids), known),
		Holdings: held,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return err
	}
	dest := filepath.Join(*root, filepath.FromSlash(holdingsOut))
	if *check {
		old, err := os.ReadFile(dest)
		if err != nil || !bytes.Equal(old, buf.Bytes()) {
			return fmt.Errorf("%s is stale: run go run ./cmd/corpus holdings", holdingsOut)
		}
		fmt.Printf("%s matches (%d labelled, %d with an origin)\n", holdingsOut, len(held), known)
		return nil
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("holdings: %d of %d rulings labelled, %d with an origin\n", len(held), len(ids), known)
	return nil
}
