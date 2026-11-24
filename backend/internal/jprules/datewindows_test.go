package jprules

import (
	"os"
	"strings"
	"testing"
)

const dateWindowsDoc = "../../../docs/japan/date-windows.md"

// Every embedded schedule window must be listed in docs/japan/date-windows.md
// with its start date, so a changed date cannot ship without the evidence table.
func TestDateWindowsDocListsEverySchedule(t *testing.T) {
	raw, err := os.ReadFile(dateWindowsDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	table := mustDefault(t)
	for _, ss := range table.schedules {
		for _, s := range ss {
			found := false
			for _, line := range strings.Split(doc, "\n") {
				if strings.HasPrefix(line, "|") && strings.Contains(line, s.Agreement) && strings.Contains(line, s.From) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("date-windows.md has no table row with %q and %s", s.Agreement, s.From)
			}
		}
	}
	for _, want := range []string{"unverified", "1231682", "2022-04-04", "2023-03-01", "2022-01-01", "indonesia-scope-notes.md", "general-notes-gap.md"} {
		if !strings.Contains(doc, want) {
			t.Errorf("date-windows.md missing %q", want)
		}
	}
}

// The Thailand start stays at the earliest start with a pinned official
// notice; the 2022-01-01 candidate is documented, not applied.
func TestThailandStartNotLoweredWithoutNotice(t *testing.T) {
	s := scheduleFrom(t, mustDefault(t), "JP-Thailand EPA", "2023-01-01")
	if s.From != "2023-01-01" {
		t.Fatalf("from = %s", s.From)
	}
}

// The README must not claim the AJCEP start follows the Customs index.
func TestReadmeAJCEPStartNotClaimedFromIndex(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	if strings.Contains(doc, "The AJCEP start dates follow the Japan Customs index") {
		t.Error("README still claims AJCEP start dates follow the Customs index")
	}
	if !strings.Contains(doc, "official start unverified") {
		t.Error("README must say the AJCEP HS 2017 official start is unverified")
	}
}

// The AJCEP HS 2017 window ends where the HS 2022 edition starts (Customs index
// "2027.2.14" and "2027.2.15~"); the start is the unverified embedded start.
func TestAJCEPHS2017WindowBounds(t *testing.T) {
	table := mustDefault(t)
	var found bool
	for _, s := range table.schedules["AJCEP"] {
		if s.From == "2023-01-01" {
			found = true
			if s.Until != "2027-02-15" {
				t.Fatalf("until = %s", s.Until)
			}
		}
	}
	if !found {
		t.Fatal("AJCEP HS 2017 schedule not found")
	}
}
