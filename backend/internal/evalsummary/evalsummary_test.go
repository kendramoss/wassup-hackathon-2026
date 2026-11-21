package evalsummary

import (
	"strings"
	"testing"
)

func TestEmbeddedSummaryLoads(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Status == "" {
		t.Error("status is empty")
	}
	for _, it := range s.Items {
		if it.Source == "" || it.Key == "" {
			t.Errorf("item without key or source: %+v", it)
		}
	}
	c := s.VerifierCalibration
	if c == nil || c.Bar != 0.92 || len(c.Splits) != 2 || c.Splits[0].Split != "dev" || c.Splits[1].Split != "final" {
		t.Fatalf("verifier calibration = %+v", c)
	}
	for _, sp := range c.Splits {
		if len(sp.Bands) != 4 || sp.PairLevel.Pairs != sp.Pairs || len(sp.Runs) == 0 {
			t.Errorf("%s: %+v", sp.Split, sp)
		}
	}
	if c.Splits[1].Caveat == "" {
		t.Error("the 2026 pairs must carry the contamination caveat")
	}
}

// The results page reads the combined post-cutoff pairs and the repeatability
// runs from here, so the page and the API say the same thing (D220).
func TestSummaryHasCombinedPairsAndRepeatability(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Item{}
	for _, it := range s.Items {
		by[it.Key] = it
	}
	for key, want := range map[string]struct {
		v float64
		n int
	}{
		"planner_post_combined_known-a_precision_at_bar": {1, 9},
		"planner_post_combined_known-a_recall_at_bar":    {0.75, 12},
		"planner_post_combined_known-a_controls_shown":   {0, 6},
		"repeat_dev_answer_flips":                        {4, 463},
	} {
		it, ok := by[key]
		if !ok || it.Value != want.v || it.N != want.n {
			t.Errorf("%s = %+v, want %v of n %d", key, it, want.v, want.n)
		}
	}
	for _, key := range []string{"repeat_dev_accuracy_run1", "repeat_dev_accuracy_run2", "repeat_dev_accuracy_run3", "repeat_dev_accuracy_mean"} {
		if it, ok := by[key]; !ok || !it.DevelopmentOnly || it.N != 463 || it.Value < 0.9 || it.Value > 0.93 {
			t.Errorf("%s = %+v", key, it)
		}
	}
}

// #543, D373: the post-cutoff flip pairs were run 3 times (D354), so no
// planner_post_ row may say it was scored once.
func TestPostCutoffPairSourcesSayThreeRuns(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, it := range s.Items {
		if !strings.HasPrefix(it.Key, "planner_post_") {
			continue
		}
		n++
		if strings.Contains(it.Source, "scored once") || !strings.Contains(it.Source, "run 1 of 3") {
			t.Errorf("%s: source %q", it.Key, it.Source)
		}
	}
	if n == 0 {
		t.Fatal("no planner_post_ rows")
	}
}
