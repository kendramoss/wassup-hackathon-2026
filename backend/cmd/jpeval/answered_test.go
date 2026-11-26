package main

import (
	"strings"
	"testing"
)

// TestAnsweredCases pins what the D921 answered splits re-check: only the
// base run's undetermined cases the card asks about, each with the product's
// question and the oracle's answer appended as the product appends them.
func TestAnsweredCases(t *testing.T) {
	want := map[string]int{"dev_eval_answered": 119, "fresh_1126_answered": 5, "dev_neg_answered": 18}
	for split, n := range want {
		items, _, _, err := loadCandidate(split)
		if err != nil {
			t.Fatal(split, err)
		}
		if len(items) != n {
			t.Fatalf("%s: %d cases, want %d", split, len(items), n)
		}
		for _, it := range items {
			f := it.caseValue.Facts
			if !strings.Contains(f, "\nQ: Which materials have no proof of origin under ") || !strings.Contains(f, "\nA: ") {
				t.Fatalf("%s %s: answer not appended: %q", split, it.label.ID, f)
			}
			if a := it.caseValue.Japan.Agreement; a == "GSP" || a == "Non-preferential" {
				t.Fatalf("%s %s: the card asks nothing under %s", split, it.label.ID, a)
			}
		}
	}
	items, _, _, err := loadCandidate("dev_neg_answered")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if !strings.Contains(it.caseValue.Facts, "imported from Brazil.") && !strings.Contains(it.caseValue.Facts, "imported from Brazil;") {
			t.Fatalf("%s: the Brazil part is not named as having no proof of origin", it.label.ID)
		}
	}
}
