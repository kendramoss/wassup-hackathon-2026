package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retrieval"
)

func TestPharmaPromptAndDatedPrecedents(t *testing.T) {
	pool := []domain.Ruling{
		{ID: "N340001", Date: day("2024-05-01"), Facts: "Metoprolol tablets: the API is made in Spain and tableted in India.",
			Holding: "The country of origin of the metoprolol tablets is Spain, where the API is made."},
		{ID: "N356001", Date: day("2026-02-01"), Facts: "Metoprolol tablets: the API is made in India and tableted in India.",
			Holding: "India."}, // dated after the case: never shown
	}
	f := llm.FakeText(`{"origin_country":"Spain","essential_character":"metoprolol API","decisive_step":"API made in Spain (N340001)","flip":"API made in India","confidence":0.96,"reasoning":"Tableting does not confer origin; see N340001 and N999999."}`)
	e := New(f, retrieval.NewBM25(pool, PrecedentText), Options{Model: "m", K: 5, System: PharmaSystem, Version: PharmaVersion})
	res, err := e.Predict(context.Background(), domain.Case{ID: "c", Date: day("2025-10-01"), Facts: "Metoprolol tablets: API from Spain, tableted in Malaysia."})
	if err != nil {
		t.Fatal(err)
	}
	p, call := res.Prediction, f.Calls()[0]
	if p.Origin != "Spain" || p.Trace.PromptVersion != PharmaVersion || res.Flip != "API made in India" {
		t.Errorf("prediction = %+v, flip %q", p, res.Flip)
	}
	if !strings.Contains(call.System, "active pharmaceutical ingredient") || strings.Contains(call.System, "102.21") {
		t.Errorf("system prompt %.80q", call.System)
	}
	user := call.Messages[0].Content
	if !strings.Contains(user, "N340001") || strings.Contains(user, "N356001") {
		t.Errorf("precedents shown: %v", p.Precedents)
	}
	if len(p.CitedRulings) != 1 || p.CitedRulings[0] != "N340001" { // N999999 was never shown
		t.Errorf("cited = %v", p.CitedRulings)
	}
}
