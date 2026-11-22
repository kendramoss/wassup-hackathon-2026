package jpcorpus

import (
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

func load(t *testing.T) *Corpus {
	t.Helper()
	c, err := Load("testdata")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCaseFactsCarryNoLabel(t *testing.T) {
	c := load(t)
	f, err := c.CaseFacts("1240001")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"walking shoes", "6404.11", "ASEAN-Japan", "Vietnam"} {
		if !strings.Contains(f, want) {
			t.Errorf("facts lack %q:\n%s", want, f)
		}
	}
	for _, leak := range []string{"Originating", "\"CC\"", "Art. 1"} {
		if strings.Contains(f, leak) {
			t.Errorf("facts leak %q (reasoning, answer or articles):\n%s", leak, f)
		}
	}
}

func TestIndexReturnsOnlyEarlierRulings(t *testing.T) {
	c := load(t)
	idx, err := c.Index([]string{"1240001", "1240002", "1250003", "1260004"})
	if err != nil {
		t.Fatal(err)
	}
	day, _ := time.Parse(time.DateOnly, "2025-06-01")
	cs := domain.Case{ID: PrecedentID("1250099"), Date: day}
	hits := idx.Top("shoes sewn in Vietnam with uppers", 5, retrieval.PrecedentFilter(cs))
	if len(hits) != 3 {
		t.Fatalf("hits = %d, want the 3 rulings before 2025-06-01", len(hits))
	}
	if hits[0].Ruling.ID != "JP1240001" {
		t.Errorf("best hit = %s, want JP1240001", hits[0].Ruling.ID)
	}
	for _, h := range hits {
		if !h.Ruling.Date.Before(day) {
			t.Errorf("hit %s is not earlier than the case", h.Ruling.ID)
		}
		if !strings.HasPrefix(h.Ruling.Holding, "Originating") {
			t.Errorf("holding %q should start with the answer", h.Ruling.Holding)
		}
	}
}

func TestEveryAgreementHasAName(t *testing.T) {
	for _, r := range load(t).Rulings() {
		if AgreementNames[r.Agreement] == "" {
			t.Errorf("%s: agreement %q has no English name", r.ID, r.Agreement)
		}
	}
}

func TestHoldingKeepsTheRuleNotTheBoilerplate(t *testing.T) {
	long := strings.Repeat("Preamble about the agreement. ", 60) + "The non-originating materials meet \"CTH\", so the good is originating. However, the rate applies only if all other requirements are met."
	got := holdingReasoning(long)
	if !strings.Contains(got, "\"CTH\"") || strings.Contains(got, "However") {
		t.Errorf("holding = %q", got)
	}
	if n := len([]rune(got)); n > HoldingRunes+3 {
		t.Errorf("holding has %d runes, cap %d", n, HoldingRunes)
	}
}
