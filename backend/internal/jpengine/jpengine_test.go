package jpengine

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func TestMissingRuleDoesNotInvokeModel(t *testing.T) {
	model := llm.FakeText(`{}`)
	eng, err := NewDefault(model, "fake", DefaultK)
	if err != nil {
		t.Fatal(err)
	}
	c := domain.Case{Facts: "Rice cooked in Vietnam for import into Japan.", Date: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)}
	for _, input := range []*domain.JapanInput{
		nil, {Agreement: "Non-preferential", HS: "1006.30"}, {Agreement: "RCEP", HS: "missing"},
		{Agreement: "RCEP", HS: "0000.00"},
	} {
		c.Japan = input
		res, err := eng.Predict(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		p := res.Prediction
		if p.Origin != "" || p.Japan.Outcome != Undetermined || p.Japan.ReviewReason == "" {
			t.Fatalf("missing-rule prediction = %+v (%+v)", p, p.Japan)
		}
	}
	if calls := model.Calls(); len(calls) != 0 {
		t.Fatalf("made %d calls without rule text", len(calls))
	}
}

func TestJapanPredictionKeepsOnlyDatedPrecedentsAndKnownCitations(t *testing.T) {
	model := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		msg := req.Messages[0].Content
		if req.SchemaName != "jp_preference" || !strings.Contains(msg, "PRODUCT RULE SCHEDULE") ||
			!strings.Contains(msg, "CTH") || !strings.Contains(msg, "tariff code: 1806.20") {
			t.Errorf("prompt missing Japan evidence or supplier code: %s", msg)
		}
		ruling := regexp.MustCompile(`JP[0-9]{7}`).FindString(msg)
		if ruling == "" {
			t.Fatal("prompt lacks a ruling ID")
		}
		return llm.Response{Text: `{"outcome":"qualifies","criterion":"CTH","origin_country":"Viet Nam","deciding_rule":"Change of heading","confidence":0.72,"reasoning":"See ` + ruling + ` and JP9999999.","cited_rulings":["` + ruling + `","JP9999999"]}`}, nil
	}}
	eng, err := NewDefault(model, "fake", DefaultK)
	if err != nil {
		t.Fatal(err)
	}
	c := domain.Case{
		Date:    time.Date(2025, 10, 1, 14, 0, 0, 0, time.UTC),
		Japan:   &domain.JapanInput{Agreement: " rcep ", HS: "1704.90-010"},
		Facts:   "Sugar confectionery manufactured and packaged in Vietnam, imported into Japan under the RCEP. Non-originating chocolate and sugar materials.",
		Product: &domain.Product{Components: []domain.Component{{Name: "chocolate", MadeIn: "Other", HTS: "1806.20"}}},
	}
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	p := res.Prediction
	if p.Origin != "Vietnam" || p.Japan.Outcome != Qualifies || p.Japan.HS != "170490010" || p.Japan.Criterion != "CTH" {
		t.Fatalf("prediction = %+v (%+v)", p, p.Japan)
	}
	if strings.Contains(p.Reasoning, "JP9999999") || len(p.CitedRulings) != 1 || len(p.Japan.PrecedentSources) != 1 {
		t.Fatalf("unfiltered citations: %+v (%+v)", p, p.Japan)
	}
	for _, id := range p.Precedents {
		r, url, ok := eng.Get(id)
		if !ok || r.Date.Format(time.DateOnly) >= "2025-10-01" ||
			!strings.Contains(r.Facts, "Regional Comprehensive Economic Partnership (RCEP)") ||
			!strings.HasPrefix(url, "https://www.customs.go.jp/") {
			t.Fatalf("invalid precedent %s (%+v, %s)", id, r, url)
		}
	}
}
