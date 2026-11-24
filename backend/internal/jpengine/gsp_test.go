package jpengine

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
)

const gspLawURL = "https://laws.e-gov.go.jp/law/344M50000040039/20230401_505M60000040029"

func TestGSPPreflightEligibleWithJapaneseEvidenceAndNoPageFragment(t *testing.T) {
	eng, model := newEngine(t)
	c := jpCase("GSP", "6404.11", "2026-11-23")
	pf := Preflight(eng.rules, c)
	if !pf.Eligible || pf.Reason != jprules.ReasonPSR || len(pf.Evidence) < 2 {
		t.Fatalf("preflight = %+v", pf)
	}
	first := pf.Evidence[0]
	if !strings.Contains(first.Text, "履物及びゲートル") || !strings.Contains(first.Text, "第六四・〇六項を除く") {
		t.Fatalf("first evidence = %+v", first)
	}
	if first.URL != gspLawURL || !strings.HasSuffix(first.Citation, ", 別表 row 305 (第六四類)") {
		t.Fatalf("first evidence source = %+v", first)
	}
	for _, e := range pf.Evidence {
		if strings.Contains(e.URL, "#") || strings.Contains(e.Citation, "PDF page") {
			t.Errorf("evidence cites a page: %+v", e)
		}
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
	for _, e := range pf.Evidence {
		if !strings.Contains(msg, "["+e.Citation+"]\n"+e.Text+"\n") {
			t.Errorf("prompt lacks evidence %q", e.Citation)
		}
	}
	if calls[0].System != systemPrompt || calls[0].SchemaName != "jp_preference" {
		t.Fatalf("request shape changed")
	}
	// Only the e-Gov law page is listed as a source, never the Unde Labs caveat.
	p := res.Prediction.Japan
	if len(p.Sources) != len(pf.Evidence)-1 {
		t.Fatalf("%d sources for %d evidence", len(p.Sources), len(pf.Evidence))
	}
	for _, s := range p.Sources {
		if s.URL != gspLawURL || strings.Contains(s.Citation, "Unde Labs") || strings.Contains(s.URL, "#") {
			t.Errorf("source %+v", s)
		}
	}
}

func TestGSPGatesNeverReachTheModel(t *testing.T) {
	cases := []struct {
		name, hs, date string
		want           jprules.Reason
	}{
		{"before the embedded window", "6404.11", "2023-03-31", jprules.ReasonNoDatedSource},
		{"no row for the goods", "0101.21", "2026-11-23", jprules.ReasonNoMatch},
		{"code in no HS 2022 subheading", "0100.99", "2026-11-23", jprules.ReasonNotInEdition},
		{"HS 2017 only code", "0410.00", "2026-11-23", jprules.ReasonNotInEdition},
		{"heading 04.10 is outside the chapter 4 row", "0410.10", "2026-11-23", jprules.ReasonNoMatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, model := newEngine(t)
			c := jpCase("GSP", tc.hs, tc.date)
			pf := Preflight(eng.rules, c)
			if pf.Eligible || pf.Reason != tc.want || pf.ReviewReason == "" {
				t.Fatalf("preflight = %+v", pf)
			}
			if _, err := eng.Predict(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if n := len(model.Calls()); n != 0 {
				t.Fatalf("%d requests", n)
			}
		})
	}
}

func TestNotInEditionReviewReasonDoesNotNameAnEdition(t *testing.T) {
	if strings.Contains(reasonNotInEdition, "2017") {
		t.Fatalf("review reason names an edition that GSP does not use: %s", reasonNotInEdition)
	}
}
