package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
)

func marketBody(facts, market string) string {
	b := map[string]any{"date": "2026-03-01", "facts": facts}
	if market != "" {
		b["jurisdiction"] = market
	}
	out, _ := json.Marshal(b)
	return string(out)
}

const (
	usFacts = "Knit polyester neck gaiters, knit in Vietnam from Chinese yarn, then imported into the United States."
	euFacts = "We need a Binding Origin Information decision: drills with Chinese motors, assembled in Poland."
)

func detail(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	d, ok := out["jurisdiction_detail"].(map[string]any)
	if !ok {
		t.Fatalf("no jurisdiction_detail: %v", out)
	}
	return d
}

// A detected non-U.S. market with no engine runs no engine and no router,
// stores no prediction and goes to review with the plain reason. The router
// label "unclear" no longer lets non-U.S. text auto-accept (D680).
func TestUnsupportedMarketNeverGetsAUSAnswer(t *testing.T) {
	for _, regime := range []string{"unclear", RegimeGeneral} {
		t.Run(regime, func(t *testing.T) {
			hs := newHarness(t, regime)
			code, out := hs.do(t, "POST", "/v1/cases/predict", marketBody(euFacts, ""))
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, out)
			}
			tr := out["triage"].(map[string]any)
			if tr["route"] != "needs_review" || tr["reason"] != "Origin rules for the European Union aren't supported yet" {
				t.Errorf("triage = %v", tr)
			}
			if o := out["prediction"].(map[string]any)["origin_country"]; o != "" {
				t.Errorf("a prediction was shown for an unsupported market: %v", o)
			}
			if hs.engine.calls != 0 {
				t.Errorf("engine called %d times", hs.engine.calls)
			}
			if out["router"] != nil || out["clarify"] != nil {
				t.Errorf("router or clarify ran: %v %v", out["router"], out["clarify"])
			}
			d := detail(t, out)
			if out["jurisdiction"] != "eu" || d["source"] != "detected" || d["supported"] != false || d["cue"] != "BOI" {
				t.Errorf("jurisdiction = %v %v", out["jurisdiction"], d)
			}
		})
	}
}

// The caller's choice wins over the text, both ways.
func TestChosenMarketWins(t *testing.T) {
	hs := newHarness(t, RegimeGeneral)
	_, out := hs.do(t, "POST", "/v1/cases/predict", marketBody(usFacts, "uk"))
	if out["triage"].(map[string]any)["reason"] != "Origin rules for the United Kingdom aren't supported yet" || hs.engine.calls != 0 {
		t.Errorf("chosen uk: %v, engine calls %d", out["triage"], hs.engine.calls)
	}
	_, out = hs.do(t, "POST", "/v1/cases/predict", marketBody(euFacts, "us"))
	if out["triage"].(map[string]any)["route"] != "auto" || detail(t, out)["source"] != "chosen" {
		t.Errorf("chosen us on EU text: %v %v", out["triage"], out["jurisdiction_detail"])
	}
	code, out := hs.do(t, "POST", "/v1/cases/predict", marketBody(usFacts, "france"))
	if code != http.StatusBadRequest {
		t.Errorf("unknown market = %d %v", code, out)
	}
}

// Text with no market cue is a U.S. case, as before D680.
func TestNoCueIsUS(t *testing.T) {
	hs := newHarness(t, RegimeGeneral)
	_, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
	d := detail(t, out)
	if out["jurisdiction"] != "us" || d["source"] != "default" || d["supported"] != true || d["follow_ups"] != true {
		t.Errorf("jurisdiction = %v %v", out["jurisdiction"], d)
	}
	if out["triage"].(map[string]any)["route"] != "auto" {
		t.Errorf("triage = %v", out["triage"])
	}
}

// U.S. and non-U.S. authority cues together: review, whatever the router
// says; a weak non-U.S. hint reviews only when the router says unclear.
func TestMarketConflictAndHint(t *testing.T) {
	conflict := "For CBP and for the Canada Border Services Agency: kettles assembled in Vietnam."
	hint := "Kettles assembled in Vietnam from Chinese elements, priced at 30 EUR each."
	for _, tc := range []struct {
		regime, facts, route, reason string
	}{
		{RegimeGeneral, conflict, "needs_review", jurisdiction.ConflictReason},
		{"unclear", hint, "needs_review", jurisdiction.HintReason("eu")},
		{RegimeGeneral, hint, "auto", ""},
	} {
		hs := newHarness(t, tc.regime)
		_, out := hs.do(t, "POST", "/v1/cases/predict", marketBody(tc.facts, ""))
		tr := out["triage"].(map[string]any)
		if tr["route"] != tc.route || (tc.reason != "" && tr["reason"] != tc.reason) {
			t.Errorf("%s / %q: triage = %v", tc.regime, tc.facts, tr)
		}
		if out["jurisdiction"] != "us" {
			t.Errorf("jurisdiction = %v", out["jurisdiction"])
		}
		// A chosen market is never second-guessed.
		_, out = hs.do(t, "POST", "/v1/cases/predict", marketBody(tc.facts, "us"))
		if tr := out["triage"].(map[string]any); tr["route"] != "auto" {
			t.Errorf("chosen us still reviewed: %v", tr)
		}
	}
}

// A registered authority answers with its own engine; with fast path 0
// every answer is reviewed, and a bar lets confident answers through.
func TestRegisteredMarketUsesItsEngine(t *testing.T) {
	for _, bar := range []float64{0, 0.9} {
		eu := &specialistFake{version: "eu-test-v1", origin: "Poland"}
		reg, err := jurisdiction.NewRegistry(jurisdiction.USAuthority, jurisdiction.Authority{Market: "eu", Engine: eu, FastPath: bar})
		if err != nil {
			t.Fatal(err)
		}
		hs := newHarnessWith(t, RegimeGeneral, func(d *Deps) { d.Jurisdictions = reg })
		_, out := hs.do(t, "POST", "/v1/cases/predict", marketBody(euFacts, ""))
		if eu.calls.Load() != 1 || hs.engine.calls != 0 {
			t.Fatalf("eu engine %d calls, U.S. engine %d", eu.calls.Load(), hs.engine.calls)
		}
		if o := out["prediction"].(map[string]any)["origin_country"]; o != "Poland" {
			t.Errorf("origin = %v", o)
		}
		want := "needs_review"
		if bar > 0 {
			want = "auto"
		}
		if r := out["triage"].(map[string]any)["route"]; r != want {
			t.Errorf("bar %v: route %v, want %s", bar, r, want)
		}
		if detail(t, out)["supported"] != true {
			t.Errorf("detail = %v", out["jurisdiction_detail"])
		}
		// Follow-ups stay U.S.-only.
		code, _ := hs.do(t, "POST", "/v1/cases/"+out["id"].(string)+"/export", `{"format":"markdown"}`)
		if code != http.StatusBadRequest {
			t.Errorf("export on a registered non-U.S. case = %d", code)
		}
	}
}

// Every follow-up is a U.S. step: an unsupported-market case refuses them
// with the plain reason, and a plan or compare call naming another market
// than the case's is refused.
func TestFollowUpsNeedASupportedMarket(t *testing.T) {
	hs := newHarness(t, RegimeGeneral)
	var hg map[string]any
	_ = json.Unmarshal([]byte(heatGunBody(t)), &hg)
	hg["jurisdiction"] = "jp"
	b, _ := json.Marshal(hg)
	_, c := hs.do(t, "POST", "/v1/cases/predict", string(b))
	id := c["id"].(string)
	for _, call := range [][2]string{
		{"/plan", `{"target":"Thailand"}`},
		{"/compare-targets", `{"targets":["Thailand","Vietnam"]}`},
		{"/export", `{"format":"markdown"}`},
		{"/watch", `{}`},
		{"/ask", `{"question":"Why?"}`},
	} {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+call[0], call[1])
		if b, _ := json.Marshal(out); code != http.StatusBadRequest || !strings.Contains(string(b), "Origin rules for Japan aren") {
			t.Errorf("%s = %d %v", call[0], code, out)
		}
	}
	_, us := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	usID := us["id"].(string)
	code, out := hs.do(t, "POST", "/v1/cases/"+usID+"/plan", `{"target":"Thailand","jurisdiction":"eu"}`)
	if code != http.StatusBadRequest {
		t.Errorf("plan with another market = %d %v", code, out)
	}
	code, out = hs.do(t, "POST", "/v1/cases/"+usID+"/plan", `{"target":"Thailand","jurisdiction":"us"}`)
	if code != http.StatusOK || out["jurisdiction"] != "us" {
		t.Errorf("plan with the case's market = %d %v", code, out["jurisdiction"])
	}
	code, out = hs.do(t, "POST", "/v1/cases/"+usID+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`)
	if code != http.StatusOK || out["jurisdiction"] != "us" {
		t.Errorf("compare = %d %v", code, out["jurisdiction"])
	}
}

// Batch rows carry the market; an unsupported one has no origin or
// confidence and counts as out of scope.
func TestBatchRowsCarryTheMarket(t *testing.T) {
	hs := newHarness(t, RegimeGeneral)
	body := `{"items":[{"name":"gaiter","facts":"` + usFacts + `"},{"name":"drill","facts":"` + euFacts + `"},{"name":"lamp","facts":"` + usFacts + `","jurisdiction":"kr"}]}`
	code, out := hs.do(t, "POST", "/v1/cases/batch", body)
	if code != http.StatusOK {
		t.Fatalf("batch = %d %v", code, out)
	}
	rows := out["rows"].([]any)
	want := []string{"us", "eu", "kr"}
	for i, r := range rows {
		row := r.(map[string]any)
		if row["jurisdiction"] != want[i] {
			t.Errorf("row %d jurisdiction = %v", i, row["jurisdiction"])
		}
		if i > 0 && (row["predicted_origin"] != nil || row["confidence"] != nil || row["out_of_scope"] != true || row["triage_route"] != "needs_review") {
			t.Errorf("row %d = %v", i, row)
		}
	}
}

func TestListJurisdictions(t *testing.T) {
	hs := newHarness(t, RegimeGeneral)
	code, out := hs.do(t, "GET", "/v1/jurisdictions", "")
	if code != http.StatusOK {
		t.Fatalf("= %d", code)
	}
	list := out["jurisdictions"].([]any)
	if len(list) != len(jurisdiction.Markets) {
		t.Fatalf("%d markets", len(list))
	}
	for _, m := range list {
		mm := m.(map[string]any)
		if (mm["code"] == "us") != (mm["supported"] == true) || (mm["code"] == "us") != (mm["follow_ups"] == true) {
			t.Errorf("market %v", mm)
		}
	}
}

type japanPredictFunc func(context.Context, domain.Case) (engine.Result, error)

func (f japanPredictFunc) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	return f(ctx, c)
}

func TestJapanCaseUsesTradeAgreementAndStaysInReview(t *testing.T) {
	calls := 0
	jp := japanPredictFunc(func(_ context.Context, c domain.Case) (engine.Result, error) {
		calls++
		if c.Japan == nil || c.Japan.Agreement != "RCEP" || c.Japan.HS != "4412.31" || c.Jurisdiction.MarketCode() != "jp" {
			t.Fatalf("Japan engine input: %+v", c)
		}
		return engine.Result{Prediction: domain.Prediction{
			Confidence: 0.99,
			Japan:      &domain.JapanResult{Agreement: c.Japan.Agreement, HS: c.Japan.HS, Outcome: "undetermined", ReviewReason: "Give the tariff codes for the materials."},
		}}, nil
	})
	reg := jurisdiction.Default()
	if err := reg.Register(jurisdiction.Authority{Market: jurisdiction.JP, Engine: jp, FastPath: 0.05}); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWith(t, RegimeGeneral, func(d *Deps) { d.Jurisdictions = reg })
	body := `{"date":"2026-03-01","facts":"Plywood made and pressed in Vietnam from imported veneers.","japan":{"agreement":"RCEP","hs":"4412.31"}}`
	code, out := h.do(t, "POST", "/v1/cases/predict", body)
	if code != http.StatusCreated || calls != 1 || h.engine.calls != 0 {
		t.Fatalf("Japan dispatch: %d %v; Japan calls %d, U.S. calls %d", code, out, calls, h.engine.calls)
	}
	check := func(out map[string]any) {
		t.Helper()
		p := out["prediction"].(map[string]any)
		if out["disclaimer"] != domain.JapanDisclaimer || out["step"] != "origin" || out["jurisdiction"] != "jp" ||
			detail(t, out)["follow_ups"] != false || out["triage"].(map[string]any)["route"] != "needs_review" ||
			p["origin_country"] != "" || p["japan_preference"].(map[string]any)["outcome"] != "undetermined" ||
			out["japan"].(map[string]any)["agreement"] != "RCEP" {
			t.Errorf("unexpected Japan response: %v", out)
		}
	}
	check(out)
	code, saved := h.do(t, "GET", "/v1/cases/"+out["id"].(string), "")
	if code != http.StatusOK {
		t.Fatalf("case read: %d %v", code, saved)
	}
	check(saved)
	code, _ = h.do(t, "POST", "/v1/cases/predict", strings.Replace(body, `"japan":`, `"jurisdiction":"us","japan":`, 1))
	if code != http.StatusBadRequest || calls != 1 || h.engine.calls != 0 {
		t.Errorf("mixed-market input got status %d, Japan calls %d, U.S. calls %d", code, calls, h.engine.calls)
	}
}
