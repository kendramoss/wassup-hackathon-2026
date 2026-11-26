package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
)

// The fresh splits (D919) are the 44 rulings Japan Customs published after
// the 22 Nov snapshot, kept in their own directory so the frozen corpus,
// splits and labels stay byte-identical. fresh_1126 retrieves precedents from
// every earlier ruling, as production does; fresh_1126_notwin drops from that
// pool each earlier ruling recorded as a near-duplicate in twins.json.
//
// Later months follow the same layout (D931): split fresh_<MMDD> reads
// data/jp/fresh-<MMDD>, written by the monthly re-scoring (docs/REFRESH.md),
// and fresh_<MMDD>_notwin drops its twins.
var freshSplit = regexp.MustCompile(`^fresh_([0-9]{4})(_notwin)?$`)

// freshDirOf names the data/jp directory of a fresh split.
func freshDirOf(split string) (string, bool) {
	m := freshSplit.FindStringSubmatch(split)
	if m == nil {
		return "", false
	}
	return "fresh-" + m[1], true
}

func isFresh(split string) bool { _, ok := freshDirOf(split); return ok }

func freshCases(c *jpcorpus.Corpus, sp jpcorpus.Splits, split string) ([]candidateInput, *jpcorpus.Pool, string, error) {
	name, _ := freshDirOf(split)
	dir := filepath.Join(dataDir, name)
	fc, err := jpcorpus.Load(dir)
	if err != nil {
		return nil, nil, "", err
	}
	drop := map[string]bool{}
	if strings.HasSuffix(split, "_notwin") {
		b, err := os.ReadFile(filepath.Join(dir, "twins.json"))
		if err != nil {
			return nil, nil, "", err
		}
		var t struct {
			Twins map[string][]struct{ ID string } `json:"twins"`
		}
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, nil, "", err
		}
		for _, ts := range t.Twins {
			for _, x := range ts {
				drop[x.ID] = true
			}
		}
	}
	var poolIDs []string
	for _, id := range append(append([]string(nil), sp.Dev...), sp.HeldOut...) {
		if !drop[id] {
			poolIDs = append(poolIDs, id)
		}
	}
	pool, err := jpcorpus.NewPool(c, poolIDs)
	if err != nil {
		return nil, nil, "", err
	}
	labelPath := filepath.Join(dir, "labels.jsonl")
	labels, err := jpcorpus.LoadLabels(labelPath)
	if err != nil {
		return nil, nil, "", err
	}
	raw, err := os.ReadFile(labelPath)
	if err != nil {
		return nil, nil, "", err
	}
	var items []candidateInput
	for _, r := range fc.Rulings() {
		label, ok := labels[r.ID]
		if !ok || label.Agreement != r.Agreement {
			return nil, nil, "", fmt.Errorf("fresh case %s is missing or its label does not match", r.ID)
		}
		facts, err := fc.CaseFacts(r.ID)
		if err != nil {
			return nil, nil, "", err
		}
		d, err := r.Time()
		if err != nil {
			return nil, nil, "", err
		}
		items = append(items, candidateInput{label: label, caseValue: domain.Case{
			ID: jpcorpus.PrecedentID(r.ID), Date: d, Facts: facts,
			Japan: &domain.JapanInput{Agreement: r.Agreement, HS: r.HS},
		}})
	}
	if len(items) != len(labels) {
		return nil, nil, "", fmt.Errorf("fresh labels have %d ids; rulings %d", len(labels), len(items))
	}
	return items, pool, fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
