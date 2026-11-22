package jpcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const repoData = "../../../data/jp"

// The committed snapshot keeps the D710 split rules: dev before 2026,
// held-out from 2026, dev_eval inside dev, no ruling in both, and labels
// for development rulings only.
func TestCommittedSplitKeepsD710(t *testing.T) {
	c, err := Load(repoData)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := LoadSplits(filepath.Join(repoData, "splits.json"))
	if err != nil {
		t.Fatal(err)
	}
	dev := map[string]bool{}
	for _, id := range sp.Dev {
		r, ok := c.Ruling(id)
		if !ok || r.Date >= "2026-01-01" {
			t.Errorf("dev ruling %s missing or dated %s", id, r.Date)
		}
		dev[id] = true
	}
	for _, id := range sp.HeldOut {
		r, ok := c.Ruling(id)
		if !ok || r.Date < "2026-01-01" || dev[id] {
			t.Errorf("held-out ruling %s missing, dated %s, or also in dev", id, r.Date)
		}
	}
	if len(sp.Dev)+len(sp.HeldOut) != c.Len() {
		t.Errorf("splits cover %d of %d rulings", len(sp.Dev)+len(sp.HeldOut), c.Len())
	}
	for _, id := range sp.DevEval {
		if !dev[id] {
			t.Errorf("dev_eval ruling %s is not in dev", id)
		}
	}
	labels, err := LoadLabels(filepath.Join(repoData, "labels-dev.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for id := range labels {
		if !dev[id] {
			t.Errorf("label for %s, which is not a development ruling", id)
		}
	}
	if _, err := os.Stat(filepath.Join(repoData, "labels-heldout.jsonl")); err == nil {
		t.Error("labels-heldout.jsonl exists before phase 2 (D710)")
	}
	for _, r := range c.Rulings() {
		if AgreementNames[r.Agreement] == "" {
			t.Errorf("%s: agreement %q has no English name", r.ID, r.Agreement)
		}
		if _, ok := c.English(r.ID); !ok {
			t.Errorf("%s has no translation", r.ID)
		}
	}
}

// Every parsed ruling names the raw page hash recorded in snapshot.json.
func TestRulingsMatchSnapshotHashes(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoData, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snap struct {
		Pages []struct {
			ID     string `json:"id"`
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatal(err)
	}
	c, err := Load(repoData)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Pages) != c.Len() {
		t.Fatalf("snapshot has %d pages, corpus %d rulings", len(snap.Pages), c.Len())
	}
	for _, p := range snap.Pages {
		r, ok := c.Ruling(p.ID)
		if !ok || r.RawSHA256 != p.SHA256 || r.URL != p.URL {
			t.Errorf("%s: ruling and snapshot disagree", p.ID)
		}
	}
}
