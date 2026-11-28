package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func committedSummary(t *testing.T) Summary {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "eval", "challenges", "specialists", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPublicFeaturesAreTheFiveCurrentPathStudiesAndSatisfyTheContract(t *testing.T) {
	s := committedSummary(t)
	want := []string{"textile_live_v2", "pharma_specialist", "part102_specialist", "japan_live_unmodified", "legal_routing"}
	if len(s.PublicFeatures) != len(want) {
		t.Fatalf("%d public features", len(s.PublicFeatures))
	}
	for i, f := range s.PublicFeatures {
		if f.ID != want[i] {
			t.Errorf("public feature %d is %s, want %s", i, f.ID, want[i])
		}
		if len(f.CandidateRevision) != 40 || len(f.DatasetSHA256) != 64 || f.SourceFamilies == 0 || f.Development.Cases == 0 || f.Test.Cases < 100 ||
			f.CostUSD <= 0 || f.LatencyP95MS == 0 || len(f.Limitations) == 0 || f.PublicNote == "" || f.Source.Decision == "" || len(f.Source.Results) == 0 {
			t.Errorf("%s: contract field missing: %+v", f.ID, f)
		}
		if f.Test.Matched+f.Test.Mismatched+f.Test.Errors != f.Test.Cases {
			t.Errorf("%s: test denominators do not add up", f.ID)
		}
		if len(f.DisplayMetrics) < 2 {
			t.Errorf("%s: too few display metrics", f.ID)
		}
		for _, m := range f.DisplayMetrics {
			if m.ID == "" || m.Label == "" || m.Den <= 0 || m.Num < 0 || m.Num > m.Den {
				t.Errorf("%s metric %+v", f.ID, m)
			}
		}
		if len(f.PublicNote) > 420 {
			t.Errorf("%s: public note is not short (%d characters)", f.ID, len(f.PublicNote))
		}
	}
}

func TestPublicNotesSayWhatTheReaderMustKnow(t *testing.T) {
	s := committedSummary(t)
	note := map[string]string{}
	for _, f := range s.PublicFeatures {
		note[f.ID] = strings.ToLower(f.PublicNote)
	}
	for id, words := range map[string][]string{
		"textile_live_v2":       {"post hoc", "already"},
		"japan_live_unmodified": {"post hoc", "already", "13", "edition", "9", "before any model"},
		"pharma_specialist":     {"113", "12", "router"},
		"part102_specialist":    {"9", "auto-accept"},
	} {
		for _, w := range words {
			if !strings.Contains(note[id], w) {
				t.Errorf("%s note lacks %q: %s", id, w, note[id])
			}
		}
	}
}

func TestPublicFeaturesLeaveTheOriginalStudyIntact(t *testing.T) {
	s := committedSummary(t)
	ids := []string{}
	for _, f := range s.Features {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "textile_specialist,pharma_specialist,part102_specialist,japan_engine" || s.Routing == nil || s.Comparator == nil || s.JapanComparator == nil {
		t.Fatalf("original objects changed: %v", ids)
	}
	for _, f := range s.Features {
		if f.Test.Cases < 100 {
			t.Errorf("%s test cases %d", f.ID, f.Test.Cases)
		}
	}
	if s.Features[0].Test.Matched != 112 || s.Features[1].Test.Matched != 113 || s.Features[2].Test.Matched != 103 || s.Features[3].Test.Matched != 104 || s.Routing.Test.Matched != 425 {
		t.Error("original study counts changed")
	}
}
