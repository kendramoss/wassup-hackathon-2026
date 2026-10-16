package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
)

// reasoningFile writes CBP's reasoning for the rulings in an ID list, so the
// server can show it with precedents without the raw corpus (textile-v2, D53).
func reasoningFile(args []string) error {
	fl := flag.NewFlagSet("reasoning", flag.ExitOnError)
	list := fl.String("ids", "data/textile_rulings.json", "ID list ({\"rulings\": [...]})")
	rawDir := fl.String("raw", "../data/corpus/raw", "raw CROSS JSON")
	out := fl.String("out", "data/textile_reasoning.json", "output: {ruling: reasoning}")
	_ = fl.Parse(args)
	ids, err := corpus.LoadIDList(*list)
	if err != nil {
		return err
	}
	res := map[string]string{}
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(*rawDir, id+".json"))
		if err != nil {
			return fmt.Errorf("read %s: %w", id, err)
		}
		var doc struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", id, err)
		}
		if r := corpus.Reasoning(doc.Text); r != "" {
			res[id] = r
		}
	}
	b, _ := json.MarshalIndent(res, "", " ")
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d of %d rulings have reasoning -> %s\n", len(res), len(ids), *out)
	return nil
}
