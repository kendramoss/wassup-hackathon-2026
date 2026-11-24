package jpengine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

const undeterminedAnswer = `{"outcome":"undetermined","criterion":"NONE","origin_country":"","deciding_rule":"missing facts","confidence":0.5,"reasoning":"Missing facts.","cited_rulings":[]}`

func newEngine(t *testing.T) (*Engine, *llm.Fake) {
	t.Helper()
	model := llm.FakeText(undeterminedAnswer)
	eng, err := NewDefault(model, "fake", DefaultK)
	if err != nil {
		t.Fatal(err)
	}
	return eng, model
}

func jpCase(agreement, hs, date string) domain.Case {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return domain.Case{
		Date:  d.Add(15 * time.Hour),
		Japan: &domain.JapanInput{Agreement: agreement, HS: hs},
		Facts: "Goods manufactured in Thailand and imported into Japan. Non-originating inputs from outside the region were used.",
	}
}

// legacyRequest rebuilds the request the pre-preflight Predict sent, using
// only Lookup and the unchanged prompt builder.
func legacyRequest(t *testing.T, e *Engine, c domain.Case) (llm.Request, bool) {
	t.Helper()
	agreement, hs := agreementCode(c.Japan.Agreement), hsNumber(c.Japan.HS)
	y, m, d := c.Date.Date()
	c.Date = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	rules, err := e.rules.Lookup(agreement, hs, c.Date)
	if err != nil || len(rules) == 0 {
		return llm.Request{}, false
	}
	facts := caseFacts(c)
	precedents := e.pool.Top(c, agreement, agreement+" "+facts, e.k)
	return llm.Request{
		Model: e.modelID, System: systemPrompt, SchemaName: "jp_preference", JSONSchema: answerSchema, MaxOutputTokens: 8192,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: userPrompt(agreement, hs, facts, rules, precedents)}},
	}, true
}

func TestPSRRequestBytesUnchanged(t *testing.T) {
	cases := []domain.Case{
		jpCase(" rcep ", "1704.90-010", "2025-10-01"),
		jpCase("AJCEP", "0901.11", "2025-10-01"),
		jpCase("AJCEP", "0101", "2023-01-01"),
		jpCase("AJCEP", "6210.50", "2026-11-23"),
		jpCase("AJCEP", "0101", "2027-02-15"), // future HS 2022 PSR
		jpCase("JP-EU EPA", "0406.90", "2025-10-01"),
		jpCase("CPTPP", "6404.11", "2025-10-01"),
		jpCase("JP-Thailand EPA", "0101", "2025-10-01"),
	}
	for _, c := range cases {
		eng, model := newEngine(t)
		want, ok := legacyRequest(t, eng, c)
		if !ok {
			t.Fatalf("legacy path has no rule for %+v", c.Japan)
		}
		if _, err := eng.Predict(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		calls := model.Calls()
		if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
			t.Fatalf("%+v: request differs from legacy\n got %d calls", c.Japan, len(calls))
		}
		if strings.Contains(calls[0].Messages[0].Content, "asean1.pdf") {
			t.Fatal("general text leaked into a PSR request")
		}
	}
}

func TestGeneralRuleCaseSendsOneRequestWithAllProvisions(t *testing.T) {
	table, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, hs := range []string{"3215", "3215.90", "3215.90-000", "9018.90"} {
		eng, model := newEngine(t)
		c := jpCase("AJCEP", hs, "2025-06-01")
		res, err := eng.Predict(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		calls := model.Calls()
		if len(calls) != 1 {
			t.Fatalf("%s: %d requests", hs, len(calls))
		}
		det, _ := table.LookupDetailed("AJCEP", hsNumber(hs), c.Date)
		if det.Reason != jprules.ReasonGeneral || len(det.Evidence) < 20 {
			t.Fatalf("%s: %+v", hs, det.Reason)
		}
		msg := calls[0].Messages[0].Content
		if !strings.Contains(msg, "PRODUCT RULE SCHEDULE") {
			t.Fatal("missing rules section")
		}
		last := -1
		for i, e := range det.Evidence {
			at := strings.Index(msg, "["+e.Citation+"]\n"+e.Text+"\n")
			if at < 0 || at < last {
				t.Fatalf("%s: evidence %d missing or out of order in prompt", hs, i)
			}
			last = at
		}
		for _, marker := range []string{
			"Article 24 Originating Goods", "Article 26 Goods Not Wholly Obtained", "Article 27 Calculation of Regional Value Content",
			"Article 28 De Minimis", "Article 29 Accumulation", "Article 30 Non-qualifying Operations",
			"Article 32 Packing Materials", "Article 33 Accessories", "Article 34 Indirect Materials", "Article 35 Identical and Interchangeable",
			"Annex 3 Information Technology Products", "Annex 2 Product Specific Rules",
			"not treaty text", "ITA Attachment A and B product lists are not embedded", "Article 31 (Direct Consignment)",
			"Annex 4 has not been verified", "not a claim of full preferential entitlement", "does not convert codes between Harmonized System editions",
		} {
			if !strings.Contains(msg, marker) {
				t.Errorf("%s: prompt lacks %q", hs, marker)
			}
		}
		p := res.Prediction.Japan
		if len(p.Sources) != len(det.Evidence) {
			t.Fatalf("sources %d vs %d", len(p.Sources), len(det.Evidence))
		}
		for i, s := range p.Sources {
			if s.Citation != det.Evidence[i].Citation || s.URL != det.Evidence[i].URL {
				t.Fatalf("source %d = %+v", i, s)
			}
		}
		// Same request shape as a PSR request: unchanged system prompt and schema.
		if calls[0].System != systemPrompt || calls[0].SchemaName != "jp_preference" || calls[0].MaxOutputTokens != 8192 {
			t.Fatalf("request shape changed: %+v", calls[0])
		}
	}
}

func TestGatedCasesNeverReachTheModel(t *testing.T) {
	cases := []struct {
		name string
		c    domain.Case
		want jprules.Reason
	}{
		{"chapter 50-63 unlisted", jpCase("AJCEP", "5001", "2025-06-01"), jprules.ReasonTooBroad}, // placeholder, set below
		{"reserved 77", jpCase("AJCEP", "7701", "2025-06-01"), jprules.ReasonInvalidCode},
		{"reserved 99", jpCase("AJCEP", "9999", "2025-06-01"), jprules.ReasonInvalidCode},
		{"reserved 98", jpCase("AJCEP", "9801.00", "2025-06-01"), jprules.ReasonInvalidCode},
		{"too broad AJCEP", jpCase("AJCEP", "0901", "2025-06-01"), jprules.ReasonTooBroad},
		{"withheld CPTPP", jpCase("CPTPP", "8708.94", "2025-06-01"), jprules.ReasonWithheld},
		{"AJCEP at HS2022 start unlisted", jpCase("AJCEP", "3215", "2027-02-15"), jprules.ReasonNoMatch},
		{"AJCEP before window", jpCase("AJCEP", "3215", "2022-12-31"), jprules.ReasonNoDatedSource},
		{"GSP", jpCase("GSP", "3215", "2025-06-01"), jprules.ReasonUnsupportedAgreement},
		{"JP-US TA", jpCase("JP-US TA", "3215", "2025-06-01"), jprules.ReasonUnsupportedAgreement},
		{"RCEP unlisted", jpCase("RCEP", "0000.00", "2025-06-01"), jprules.ReasonNoMatch},
		{"RCEP before start", jpCase("RCEP", "6404.11", "2022-12-31"), jprules.ReasonNoDatedSource},
		{"CPTPP unlisted", jpCase("CPTPP", "0304", "2025-06-01"), jprules.ReasonTooBroad},
		{"no HS", jpCase("AJCEP", "", "2025-06-01"), ReasonNoHS},
		{"bad HS", jpCase("AJCEP", "32", "2025-06-01"), ReasonNoHS},
		{"no agreement", jpCase("", "3215", "2025-06-01"), ReasonNoAgreement},
		{"non-preferential", jpCase("Non-preferential", "3215", "2025-06-01"), ReasonNonPreferential},
	}
	// Find a real unlisted chapter 50-63 heading rather than assuming one.
	table, _ := jprules.Default()
	for h := 5000; h < 6400; h++ {
		code := []byte("0000")
		for i, v := 0, h; i < 4; i, v = i+1, v/10 {
			code[3-i] = byte('0' + v%10)
		}
		if res, _ := table.LookupDetailed("AJCEP", string(code), time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)); res.Reason == jprules.ReasonExcludedGeneral {
			cases[0].c = jpCase("AJCEP", string(code), "2025-06-01")
			cases[0].want = jprules.ReasonExcludedGeneral
			break
		}
	}
	if cases[0].want != jprules.ReasonExcludedGeneral {
		t.Fatal("no excluded textile heading found")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, model := newEngine(t)
			pf := Preflight(eng.rules, tc.c)
			if pf.Eligible || pf.Reason != tc.want || pf.Evidence != nil || pf.ReviewReason == "" {
				t.Fatalf("preflight = %+v, want reason %q", pf, tc.want)
			}
			res, err := eng.Predict(context.Background(), tc.c)
			if err != nil {
				t.Fatal(err)
			}
			if p := res.Prediction.Japan; p.Outcome != Undetermined || p.ReviewReason != pf.ReviewReason || len(p.Sources) != 0 {
				t.Fatalf("prediction = %+v", p)
			}
			if n := len(model.Calls()); n != 0 {
				t.Fatalf("%d requests", n)
			}
		})
	}
}

func TestPreflightMatchesPredictOrderAndNormalization(t *testing.T) {
	eng, model := newEngine(t)
	if pf := Preflight(eng.rules, domain.Case{}); pf.Reason != ReasonNoFacts || pf.Eligible {
		t.Fatalf("%+v", pf)
	}
	if _, err := eng.Predict(context.Background(), domain.Case{}); !errors.Is(err, engine.ErrNoFacts) {
		t.Fatalf("err = %v", err)
	}
	// Facts are checked before the date.
	if _, err := eng.Predict(context.Background(), domain.Case{Date: time.Now()}); !errors.Is(err, engine.ErrNoFacts) {
		t.Fatalf("err = %v", err)
	}
	c := jpCase("AJCEP", "3215", "2025-06-01")
	c.Date = time.Time{}
	if pf := Preflight(eng.rules, c); pf.Reason != ReasonNoDate {
		t.Fatalf("%+v", pf)
	}
	if _, err := eng.Predict(context.Background(), c); err == nil || !strings.Contains(err.Error(), "case date is required") {
		t.Fatalf("err = %v", err)
	}
	c = jpCase(" ajcep ", " 3215.90 ", "2025-06-01")
	c.Date = time.Date(2025, 6, 1, 23, 59, 0, 0, time.FixedZone("x", 9*3600))
	pf := Preflight(eng.rules, c)
	if !pf.Eligible || pf.Reason != jprules.ReasonGeneral || pf.Agreement != "AJCEP" || pf.HS != "321590" ||
		!pf.Date.Equal(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%+v", pf)
	}
	if len(model.Calls()) != 0 {
		t.Fatal("Preflight called the model")
	}
	// Existing review wording is unchanged.
	for _, tc := range []struct {
		c    domain.Case
		want string
	}{
		{jpCase("", "3215", "2025-06-01"), "Choose the trade agreement you want to use for this import."},
		{jpCase("Non-preferential", "3215", "2025-06-01"), "This check covers preferences under trade agreements. Ask a specialist about Japan's other origin rules."},
		{jpCase("GSP", "3215", "2025-06-01"), "We do not have a verified product rule for this trade agreement."},
		{jpCase("AJCEP", "", "2025-06-01"), "Enter the finished product's tariff code. Use at least four digits."},
		{jpCase("AJCEP", "9999", "2025-06-01"), "We could not verify a rule for this tariff code and date. Check the agreement's tariff schedule or ask a specialist."},
		{jpCase("CPTPP", "8708.94", "2025-06-01"), "We could not verify a rule for this tariff code and date. Check the agreement's tariff schedule or ask a specialist."},
	} {
		if got := Preflight(eng.rules, tc.c).ReviewReason; got != tc.want {
			t.Errorf("review reason = %q, want %q", got, tc.want)
		}
	}
}
