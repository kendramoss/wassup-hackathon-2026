package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evalFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "eval", "tariffwatch", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExclusionNumbersKeepOnlyTheNumbersInForce(t *testing.T) {
	ocr := `5) | 0304.83.5020
Pump casings (described in statistical reporting number 8413.91.9080
prior to January 1, 2019; described in statistical reporting number 8413.91.9095
effective January 1, 2019 through December 31, 2019; described in statistical reporting
number 8413.91.9085 or 8413.91.9096 effective January 1, 2020)
Air purification (described in statistical reporting number 8421.39.8015 prior to January 27, 2022; described in statistical reporting number 8421.39.0115 effective January 27, 2022)
Federal Register / Vol. 89 | 46951
7) | Compressors (described in statistical reporting number 8414.30.8030)
Screens (described in statistical reporting number 9013.80.9000 prior to January 27, 2022, described in statistical reporting number 8524.11.9000 effective January 27, 2022)
Mats (described in statistical reporting number 5603.92.0090 prior to July 1, 2022; 5603.92.0070 or 5603.92.0095 effective July 1, 2022)
`
	got := strings.Join(exclusionNumbers(ocr), " ")
	want := "0304.83.5020 5603.92.0070 5603.92.0095 8413.91.9085 8413.91.9096 8414.30.8030 8421.39.0115 8524.11.9000"
	if got != want {
		t.Errorf("numbers = %s\nwant      %s", got, want)
	}
}

func TestVotedNumbersNeedTwoReadings(t *testing.T) {
	got := votedNumbers([]string{"a", "b"}, []string{"a", "c"}, []string{"a", "b", "d"})
	if strings.Join(got, " ") != "a b" {
		t.Errorf("voted = %v", got)
	}
}

func TestCommittedTruthSources(t *testing.T) {
	ex := votedNumbers(exclusionNumbers(evalFile(t, "fr-89FR46948-annexC.ocr-scale3.txt")), exclusionNumbers(evalFile(t, "fr-89FR46948-annexC.ocr-scale4.txt")), exclusionNumbers(evalFile(t, "fr-89FR46948-annexC.ocr-scale6.txt")))
	if len(ex) < 120 || len(subheadings(ex)) < 100 {
		t.Errorf("%d numbers, %d subheadings read from the OCR of 89 FR 46948", len(ex), len(subheadings(ex)))
	}
	solar := solarNumbers(evalFile(t, "fr-89FR76581-annexB.txt"))
	if strings.Join(subheadings(solar), " ") != "8486.10.00 8486.20.00 8486.40.00" {
		t.Errorf("solar = %v", solar)
	}
	lines, rate := maritimeLines(evalFile(t, "fr-90FR48320-annexVA.txt"))
	if strings.Join(subheadings(lines), " ") != "8426.19.00 8716.39.00 8716.90.30 8716.90.50" || rate != 1 {
		t.Errorf("maritime = %v at %v", lines, rate)
	}
}

func TestCommittedSummaryIsWhatTheDataGives(t *testing.T) {
	got, err := runEval(filepath.Join("..", "..", "..", "eval", "tariffwatch"), filepath.Join("..", "..", "internal", "tariffwatch", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "..", "eval", "tariffwatch", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(got, "", " ")
	if strings.TrimSpace(string(b)) != strings.TrimSpace(string(want)) {
		t.Error("eval/tariffwatch/summary.json differs from the recomputed result; run go run ./cmd/tariffwatch eval -write")
	}
	d2 := got.Held.D2
	if d2.Recall < 0.95 || d2.Precision < 0.90 || got.Held.Rate.MaritimeExact != got.Held.Rate.MaritimeN || got.Dev.AffectedD1 != 0 || got.Dev.AffectedD2 != 0 {
		t.Errorf("D941 bars not met: %+v", got)
	}
}
