package jpengine

import (
	"context"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
)

func TestJPUSPreflightEligibleWithJapaneseEvidence(t *testing.T) {
	eng, model := newEngine(t)
	c := jpCase("JP-US TA", "0406.90", "2026-11-23")
	pf := Preflight(eng.rules, c)
	if !pf.Eligible || pf.Reason != jprules.ReasonPSR || len(pf.Evidence) < 2 {
		t.Fatalf("preflight = %+v", pf)
	}
	if !strings.Contains(pf.Evidence[0].Text, "ＣＣ（第一九〇一・九〇号の酪農調製品") {
		t.Fatalf("first evidence = %+v", pf.Evidence[0])
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
	// Only the treaty pages are listed as sources, never the Unde Labs caveat.
	p := res.Prediction.Japan
	if len(p.Sources) != len(pf.Evidence)-1 {
		t.Fatalf("%d sources for %d evidence", len(p.Sources), len(pf.Evidence))
	}
	for _, s := range p.Sources {
		if !strings.HasPrefix(s.URL, "https://www.customs.go.jp/roo/text/us2.pdf#page=") || strings.Contains(s.Citation, "Unde Labs") {
			t.Errorf("source %+v", s)
		}
	}
}

func TestJPUSGatesNeverReachTheModel(t *testing.T) {
	cases := []struct {
		name, hs, date string
		want           jprules.Reason
	}{
		{"outside the listed goods", "0301.11", "2026-11-23", jprules.ReasonNoMatch},
		{"national item is too broad", "2206.00", "2026-11-23", jprules.ReasonTooBroad},
		{"before entry into force", "0406.90", "2019-12-31", jprules.ReasonNoDatedSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, model := newEngine(t)
			c := jpCase("JP-US TA", tc.hs, tc.date)
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
