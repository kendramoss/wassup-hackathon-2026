package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/hts"
)

// htsIndex writes the rulings whose text states the HTS subheading CBP
// applied, so the server can suggest subheadings without the raw corpus
// (D64).
func htsIndex(args []string) error {
	fl := flag.NewFlagSet("hts-index", flag.ExitOnError)
	labelsPath := fl.String("labels", "data/labels.jsonl", "labels file listing the rulings (order, date, subject)")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON")
	out := fl.String("out", "data/hts_index.json", "output index")
	_ = fl.Parse(args)
	labels, err := corpus.LoadLabels(*labelsPath)
	if err != nil {
		return err
	}
	x := hts.Index{Note: "Rulings whose text states the HTSUS subheading CBP applied, 8 digits, chapters 98/99 excluded; built by `go run ./cmd/corpus hts-index` from the raw CROSS texts (D64).", Rulings: []hts.Entry{}}
	bySource := map[string]int{}
	missing := 0
	for _, l := range labels {
		b, err := os.ReadFile(filepath.Join(*rawDir, l.Ruling+".json"))
		if errors.Is(err, fs.ErrNotExist) {
			missing++
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", l.Ruling, err)
		}
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", l.Ruling, err)
		}
		codes, src := hts.Extract(doc.Text)
		if len(codes) == 0 {
			continue
		}
		x.Rulings = append(x.Rulings, hts.Entry{Ruling: l.Ruling, Date: l.Date, Subject: l.Subject, HTS: codes, Source: src})
		bySource[src]++
	}
	if missing == len(labels) {
		return fmt.Errorf("no raw rulings in %s (run: go run ./cmd/corpus fetch)", *rawDir)
	}
	b, _ := json.MarshalIndent(x, "", " ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d of %d rulings state a subheading (%d distinct; by source %v; %d raw files missing) -> %s\n",
		len(x.Rulings), len(labels), x.Subheadings(), bySource, missing, *out)
	return nil
}

// partHTSIndex writes every sentence of the raw corpus that classifies a
// named part or good (hts.PartStatements), with its ruling and date, so the
// server can match a part of a user's product to parts CBP classified before
// (D380). Every raw ruling is read, Part 102 rulings included; the
// evaluation (D381) drops a ruling's own sentences and every later ruling.
func partHTSIndex(args []string) error {
	fl := flag.NewFlagSet("part-hts-index", flag.ExitOnError)
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON")
	out := fl.String("out", "data/part_hts_index.json", "output index")
	_ = fl.Parse(args)
	files, err := filepath.Glob(filepath.Join(*rawDir, "*.json"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no raw rulings in %s (run: go run ./cmd/corpus fetch)", *rawDir)
	}
	sort.Strings(files)
	x := hts.PartIndex{Note: "Sentences of CBP rulings that classify a named part or good (\"the aluminum pegs will be classified in subheading 8302.50\"), codes at the level stated (4, 6 or 8 digits), chapters 98/99 excluded; built by `go run ./cmd/corpus part-hts-index` from the raw CROSS texts (D380).", Entries: []hts.PartEntry{}}
	rulings := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var doc struct {
			RulingNumber string `json:"rulingNumber"`
			RulingDate   string `json:"rulingDate"`
			Text         string `json:"text"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", f, err)
		}
		st := hts.PartStatements(doc.Text)
		if len(st) == 0 || doc.RulingNumber == "" || len(doc.RulingDate) < 10 {
			continue
		}
		rulings++
		for _, s := range st {
			x.Entries = append(x.Entries, hts.PartEntry{Ruling: doc.RulingNumber, Date: doc.RulingDate[:10], Statement: s})
		}
	}
	b, _ := json.MarshalIndent(x, "", " ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d classifying sentences in %d of %d rulings -> %s\n", len(x.Entries), rulings, len(files), *out)
	return nil
}
