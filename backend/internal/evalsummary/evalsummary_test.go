package evalsummary

import "testing"

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
