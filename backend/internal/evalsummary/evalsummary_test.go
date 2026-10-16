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
}
