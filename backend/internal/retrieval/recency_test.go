package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestRecencyRerankReordersNearTiesOnly(t *testing.T) {
	c := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	old := domain.Ruling{ID: "N300000", Date: c.AddDate(-3, 0, 0)}
	recent := domain.Ruling{ID: "N350000", Date: c.AddDate(0, -1, 0)}
	far := domain.Ruling{ID: "N340000", Date: c.AddDate(0, -1, 0)}
	// A near tie: the recent ruling overtakes the old one.
	got := RecencyRerank([]Hit{{old, 10.0}, {recent, 9.5}}, c, 2)
	if got[0].Ruling.ID != "N350000" {
		t.Errorf("near tie: got %s first", got[0].Ruling.ID)
	}
	// A clear lead stays: 10% cannot close a 30% gap.
	got = RecencyRerank([]Hit{{old, 10.0}, {far, 7.0}}, c, 1)
	if got[0].Ruling.ID != "N300000" {
		t.Errorf("clear lead: got %s first", got[0].Ruling.ID)
	}
}

func TestRecencyWithoutDateIsBM25(t *testing.T) {
	docs := []domain.Ruling{{ID: "N300001", Facts: "steel bolts threaded"}, {ID: "N300002", Facts: "cotton shirts"}}
	bm := NewBM25(docs, func(r domain.Ruling) string { return r.Facts })
	r := &Recency{BM25: bm}
	all := func(domain.Ruling) bool { return true }
	a, _ := r.Search(context.Background(), "steel bolts", 1, all)
	b := bm.Top("steel bolts", 1, all)
	if len(a) != 1 || a[0].Ruling.ID != b[0].Ruling.ID {
		t.Errorf("without a case date: %v, want %v", a, b)
	}
}
