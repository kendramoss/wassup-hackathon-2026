package jpengine

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
)

func TestMappedCodeIsEligibleAndKeepsTheOriginalCode(t *testing.T) {
	eng, model := newEngine(t)
	c := jpCase("JP-EU EPA", "2404.11", "2025-06-01")
	pf := Preflight(eng.rules, c)
	if !pf.Eligible || pf.Reason != jprules.ReasonMapped || pf.HS != "240411" || pf.ReviewReason != "" {
		t.Fatalf("preflight = %+v", pf)
	}
	if len(pf.Evidence) < 2 || pf.Evidence[0].URL != "" || !strings.Contains(pf.Evidence[0].Text, "administrative correlation") {
		t.Fatalf("evidence = %+v", pf.Evidence)
	}
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	calls := model.Calls()
	if len(calls) != 1 {
		t.Fatalf("%d requests", len(calls))
	}
	msg := calls[0].Messages[0].Content
	for _, want := range []string{"FINISHED GOOD TARIFF CODE: 240411", "Unde Labs note (not treaty text)", "2403.91 and 2403.99", "CTH; MaxNOM 35 % (EXW); or RVC 70 % (FOB)."} {
		if !strings.Contains(msg, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if calls[0].System != systemPrompt || calls[0].SchemaName != "jp_preference" {
		t.Fatalf("request shape changed")
	}
	p := res.Prediction.Japan
	if p.HS != "240411" || len(p.Sources) == 0 {
		t.Fatalf("prediction = %+v", p)
	}
	for _, s := range p.Sources {
		if s.URL == "" || strings.Contains(s.Citation, "Unde Labs") || strings.Contains(s.Citation, "WCO") {
			t.Errorf("source %+v", s)
		}
	}
}

func TestEditionHintStopsBeforeTheModelWithASpecificReason(t *testing.T) {
	eng, model := newEngine(t)
	c := jpCase("CPTPP", "0307.43", "2025-06-01")
	pf := Preflight(eng.rules, c)
	if pf.Eligible || pf.Reason != jprules.ReasonEditionHint || pf.Evidence != nil {
		t.Fatalf("preflight = %+v", pf)
	}
	for _, want := range []string{"0307.49", "0307.99", "differ", "specialist"} {
		if !strings.Contains(pf.ReviewReason, want) {
			t.Errorf("review reason lacks %q: %s", want, pf.ReviewReason)
		}
	}
	if pf.ReviewReason == reasonUnverifiedRule {
		t.Error("generic reason used")
	}
	res, err := eng.Predict(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if p := res.Prediction.Japan; p.Outcome != Undetermined || p.ReviewReason != pf.ReviewReason || p.HS != "030743" || len(p.Sources) != 0 {
		t.Fatalf("prediction = %+v", p)
	}
	if len(model.Calls()) != 0 {
		t.Fatal("model was called")
	}
}

func TestUnmappedSiblingsKeepTheGenericReason(t *testing.T) {
	eng, _ := newEngine(t)
	for _, hs := range []string{"2404.12", "2404.19", "2404.91", "2404.99"} {
		pf := Preflight(eng.rules, jpCase("JP-EU EPA", hs, "2025-06-01"))
		if pf.Eligible || pf.Reason != jprules.ReasonNoMatch || pf.ReviewReason != reasonUnverifiedRule {
			t.Errorf("%s: %+v", hs, pf)
		}
	}
}
