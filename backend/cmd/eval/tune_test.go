package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
)

func TestTuneSplitExcludesEvalAndLaterRulings(t *testing.T) {
	str := func(s string) *string { return &s }
	labels := []corpus.Label{
		{Ruling: "N100001", Date: "2024-01-01", Regime: "substantial_transformation", PrimaryOrigin: str("China")},
		{Ruling: "N100002", Date: "2024-02-01", Regime: "single_country", PrimaryOrigin: str("China")},
		{Ruling: "N100003", Date: "2024-03-01", Regime: "substantial_transformation", PrimaryOrigin: nil},
		{Ruling: "N100004", Date: "2025-06-01", Regime: "substantial_transformation", PrimaryOrigin: str("Vietnam")}, // the dev ruling
		{Ruling: "N100005", Date: "2025-07-01", Regime: "substantial_transformation", PrimaryOrigin: str("Mexico")},  // after the cutoff
		{Ruling: "N100006", Date: "2024-04-01", Regime: "substantial_transformation", PrimaryOrigin: str("Japan")},
	}
	dir := t.TempDir()
	for _, l := range labels {
		_ = os.WriteFile(filepath.Join(dir, l.Ruling+".txt"), []byte("facts"), 0o644)
	}
	store, err := corpus.Load(labels, dir)
	if err != nil {
		t.Fatal(err)
	}
	train, check, cutoff, err := tuneSplit(labels, store, corpus.Splits{Dev: []string{"N100004"}})
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Sorted(slices.Values(slices.Concat(train, check)))
	if !slices.Equal(got, []string{"N100001", "N100006"}) || cutoff.Format("2006-01-02") != "2025-06-01" {
		t.Errorf("eligible = %v, cutoff %s", got, cutoff)
	}
}

func TestTuneTarget(t *testing.T) {
	r := domain.Ruling{ID: "N100001", Origin: "China", Holding: "The country of origin is China."}
	right := evalkit.Row{Pred: "china", Confidence: 0.97, KeyFactors: []string{"motor"}, DecisiveStep: "motor made in China", Flip: "make the motor in Vietnam"}
	got := tuneTargetFor(right, r, "CBP reasoning.")
	if got.Origin != "China" || got.DecisiveStep != right.DecisiveStep || got.Confidence != 0.97 || got.Flip == nil || got.Reasoning != "CBP reasoning." || got.EssentialCharacter != "motor" {
		t.Errorf("right answer target = %+v", got)
	}
	wrong := right
	wrong.Pred = "Vietnam"
	got = tuneTargetFor(wrong, r, "CBP reasoning.")
	if got.Origin != "China" || got.DecisiveStep != r.Holding || got.Flip != nil || got.Confidence != absentCorrectedConfidence {
		t.Errorf("corrected target = %+v", got)
	}
	// The 0.97 on the wrong row is confidence in Vietnam, not in CBP's China.
	if correctedTargetConfidence(wrong, "China") != absentCorrectedConfidence {
		t.Errorf("wrong-origin confidence was reused: %v", correctedTargetConfidence(wrong, "China"))
	}
	if correctedTargetConfidence(right, "China") != 0.97 {
		t.Errorf("engine confidence for CBP's origin = %v", correctedTargetConfidence(right, "China"))
	}
}
