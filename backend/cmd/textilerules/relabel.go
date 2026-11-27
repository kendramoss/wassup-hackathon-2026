package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
)

// textileLabelsFile holds the label of every ruling of the textile sets under
// the textile labeller (D957): Hong Kong is a place of its own. cmdEval reads
// it first and falls back to the older labels files.
const textileLabelsFile = "../eval/textile102/labels-textile.jsonl"

// TextileLabel is one row of labels-textile.jsonl.
type TextileLabel struct {
	corpus.Label
	Set string `json:"set"`
}

// relabelRuling labels one raw CROSS file with the textile labeller.
func relabelRuling(rawDirs []string, id string) (corpus.Label, error) {
	for _, d := range rawDirs {
		b, err := os.ReadFile(filepath.Join(d, id+".json"))
		if err != nil {
			continue
		}
		var r cross.Ruling
		if err := json.Unmarshal(b, &r); err != nil {
			return corpus.Label{}, fmt.Errorf("%s: %w", id, err)
		}
		_, meta := corpus.SplitFacts(r.Text)
		return corpus.LabelHoldingTextile(r.RulingNumber, r.RulingDate, r.Collection, r.Subject, r.Text, meta), nil
	}
	return corpus.Label{}, fmt.Errorf("%s: no raw text in %v", id, rawDirs)
}

// cmdRelabel writes labels-textile.jsonl for the named sets from the raw
// CROSS rulings (not committed) and prints how many labels differ from the
// legacy labels the sets were first built on.
//
//	go run ./cmd/textilerules relabel -raw DIR,DIR -sets dev,final,final2,older,pool
func cmdRelabel(args []string) error {
	fl := flag.NewFlagSet("relabel", flag.ExitOnError)
	raw := fl.String("raw", "", "comma-separated directories of raw CROSS JSON")
	sets := fl.String("sets", "dev,final,final2,older,pool", "sets to label")
	out := fl.String("out", textileLabelsFile, "output file")
	_ = fl.Parse(args)
	dirs := strings.Split(*raw, ",")
	gold, err := loadGold(false)
	if err != nil {
		return err
	}
	var rows []TextileLabel
	for _, s := range strings.Split(*sets, ",") {
		ids, err := splitIDs(s)
		if err != nil {
			return err
		}
		same, diff, none := 0, 0, 0
		for _, id := range ids {
			l, err := relabelRuling(dirs, id)
			if err != nil {
				return err
			}
			rows = append(rows, TextileLabel{Label: l, Set: s})
			switch old := gold[id].Origin; {
			case l.Origin() == "":
				none++
			case l.Origin() == old:
				same++
			default:
				diff++
				fmt.Printf("  %s %-7s old=%-14s new=%s\n", s, id, old, l.Origin())
			}
		}
		fmt.Printf("%-7s %d rulings: %d same, %d changed, %d now without a single origin\n", s, len(ids), same, diff, none)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Set < rows[j].Set })
	var buf []byte
	for _, r := range rows {
		b, _ := json.Marshal(r)
		buf = append(buf, append(b, '\n')...)
	}
	return os.WriteFile(*out, buf, 0o644)
}
