package adjudicate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

const facts = "The housing and the PCBA are made in China. Final assembly and testing take place in Vietnam."

func TestCandidates(t *testing.T) {
	narrow := Options{Mode: Narrow, Below: 0.96}
	sym := Options{Mode: Symmetric, Below: 0.95}
	for _, tt := range []struct {
		name  string
		facts string
		pred  string
		conf  float64
		opts  Options
		other string
		ok    bool
	}{
		{"narrow, assembly country predicted", facts, "Vietnam", 0.9, narrow, "Vietnam", true},
		{"narrow, China predicted", facts, "China", 0.9, narrow, "", false},
		{"narrow, confident", facts, "Vietnam", 0.96, narrow, "", false},
		{"no China in facts", "Made and assembled in Vietnam.", "Vietnam", 0.5, narrow, "", false},
		{"symmetric, China predicted: other is the most named", facts + " Vietnam workers pack it.", "China", 0.9, sym, "Vietnam", true},
		{"symmetric, other predicted", facts, "Vietnam", 0.94, sym, "Vietnam", true},
		{"symmetric, confident", facts, "China", 0.95, sym, "", false},
	} {
		other, ok := Candidates(tt.facts, tt.pred, tt.conf, tt.opts)
		if other != tt.other || ok != tt.ok {
			t.Errorf("%s: got %q %v, want %q %v", tt.name, other, ok, tt.other, tt.ok)
		}
	}
}

func TestApply(t *testing.T) {
	if o, c, ch := Apply("Vietnam", 0.9, "Vietnam", Decision{Origin: "China", Confidence: 0.8}); o != "China" || c != 0.8 || !ch {
		t.Errorf("override: %s %v %v", o, c, ch)
	}
	if o, c, ch := Apply("Vietnam", 0.7, "Vietnam", Decision{Origin: "China", Confidence: 0.95}); o != "China" || c != 0.7 || !ch {
		t.Errorf("override keeps the lower confidence: %s %v %v", o, c, ch)
	}
	if _, _, ch := Apply("Vietnam", 0.9, "Vietnam", Decision{Origin: "Vietnam", Confidence: 0.99}); ch {
		t.Error("agreement must not change the row")
	}
	if o, _, ch := Apply("Vietnam", 0.9, "Vietnam", Decision{Origin: "Mexico"}); o != "Vietnam" || ch {
		t.Error("an answer outside the two candidates is ignored")
	}
}

type recorder struct{ req llm.Request }

func (r *recorder) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	r.req = req
	return llm.Response{Text: `{"substantial_transformation_outside_china": false, "origin_country": "China", "confidence": 0.8, "closest_precedent": "N100001", "reasoning": "r"}`}, nil
}
func (r *recorder) Provider() string { return "fake" }

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

// Precedents are split by CBP's holding, dated strictly before the case, and
// the engine's answer is never shown.
func TestDecideShowsContrastiveEarlierPrecedents(t *testing.T) {
	pool := []domain.Ruling{
		{ID: "N100001", Date: day("2025-01-01"), Origin: "China", Facts: "PCBA made in China, assembled in Thailand.", Holding: "China"},
		{ID: "N100002", Date: day("2025-01-02"), Origin: "Thailand", Facts: "PCBA made in China, assembled in Thailand.", Holding: "Thailand"},
		{ID: "N100003", Date: day("2026-05-01"), Origin: "China", Facts: "PCBA made in China, assembled in Thailand.", Holding: "China (later)"},
		{ID: "N100004", Date: day("2025-01-03"), Origin: "China", Facts: "Made in China only.", Holding: "China"},
	}
	rec := &recorder{}
	a := New(rec, pool, Options{Model: "m", Mode: Narrow, Below: 0.96})
	d, err := a.Decide(context.Background(), domain.Case{ID: "N200000", Date: day("2026-01-01"), Facts: facts}, "Vietnam")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Held, ",") != "N100001" || strings.Join(d.Conferred, ",") != "N100002" {
		t.Fatalf("groups = %v / %v", d.Held, d.Conferred)
	}
	msg := rec.req.Messages[0].Content
	if !strings.Contains(msg, "CANDIDATES: China or Vietnam") || strings.Contains(msg, "N100003") || strings.Contains(msg, "N100004") {
		t.Errorf("user message:\n%s", msg)
	}
	if d.Origin != "China" || d.Confidence != 0.8 {
		t.Errorf("decision = %+v", d)
	}
}
