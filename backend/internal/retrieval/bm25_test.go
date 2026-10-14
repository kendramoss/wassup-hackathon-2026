package retrieval

import (
	"reflect"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func sample() []domain.Ruling {
	return []domain.Ruling{
		{ID: "N100001", Date: day("2024-01-10"), Facts: "Heat gun assembled in Thailand with motor and heating element from China."},
		{ID: "N100002", Date: day("2025-03-01"), Facts: "Hair dryer motor fan assembly made in Vietnam, housing moulded in Vietnam."},
		{ID: "N100003", Date: day("2025-12-01"), Facts: "Frozen shrimp peeled and cooked in Indonesia."},
		{ID: "N100004", Date: day("2026-02-01"), Facts: "Heat gun motor, heating element and PCBA made in Thailand."},
		{ID: "N100005", Date: day("2024-06-01"), Facts: "Unrelated chemical resin."},
	}
}

func facts(r domain.Ruling) string { return r.Facts }

func TestTokens(t *testing.T) {
	got := Tokens("The Heat-gun's PCBA, made in Thailand and the US; it has 2 motors.")
	want := []string{"heat", "gun", "pcba", "made", "thailand", "motors"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokens = %v, want %v", got, want)
	}
}

func TestSearchRanksAndFiltersByDate(t *testing.T) {
	idx := NewBM25(sample(), facts)
	c := domain.Case{ID: "N357771", Date: day("2026-01-14")}
	hits := idx.Search("heat gun with Chinese motor and heating element", 3, PrecedentFilter(c))
	if len(hits) != 3 {
		t.Fatalf("hits = %d", len(hits))
	}
	if hits[0].Ruling.ID != "N100001" {
		t.Errorf("best hit = %s, want N100001", hits[0].Ruling.ID)
	}
	for _, h := range hits {
		if !h.Ruling.Date.Before(c.Date) {
			t.Errorf("hit %s dated %s is not before the case", h.Ruling.ID, h.Ruling.Date.Format(time.DateOnly))
		}
	}
	for i := 1; i < len(hits); i++ {
		if hits[i].Score > hits[i-1].Score {
			t.Errorf("hits not sorted by score: %v", hits)
		}
	}
}

func TestSearchExcludesTheCaseItself(t *testing.T) {
	idx := NewBM25(sample(), facts)
	c := domain.Case{ID: "N100001", Date: day("2030-01-01")}
	for _, h := range idx.Search("heat gun", 5, PrecedentFilter(c)) {
		if h.Ruling.ID == "N100001" {
			t.Fatal("case retrieved as its own precedent")
		}
	}
}

func TestTiesBreakByHigherIndex(t *testing.T) {
	rs := []domain.Ruling{{ID: "N1"}, {ID: "N2"}, {ID: "N3"}}
	idx := NewBM25(rs, func(domain.Ruling) string { return "" })
	var ids []string
	for _, h := range idx.Search("anything", 3, func(domain.Ruling) bool { return true }) {
		ids = append(ids, h.Ruling.ID)
	}
	if !reflect.DeepEqual(ids, []string{"N3", "N2", "N1"}) {
		t.Errorf("tie order = %v, want [N3 N2 N1] (Python reverse sort)", ids)
	}
}

func TestSearchHonoursExcludeList(t *testing.T) {
	idx := NewBM25(sample(), facts)
	c := domain.Case{ID: "X", Date: day("2030-01-01"), Exclude: []string{"N100001"}}
	for _, h := range idx.Search("heat gun", 5, PrecedentFilter(c)) {
		if h.Ruling.ID == "N100001" {
			t.Fatal("excluded ruling retrieved")
		}
	}
}
