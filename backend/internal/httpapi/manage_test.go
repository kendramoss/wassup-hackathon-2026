package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

var admin = map[string]string{"X-Admin-Token": testAdmin}

func TestRateLimitPerClientWithAdminBypass(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{LightPerMin: 2, HeavyPerMin: 1, GlobalPerMin: 100}
	})
	body := heatGunBody(t)
	alice, bob := map[string]string{"X-Client-IP": "203.0.113.1"}, map[string]string{"X-Client-IP": "203.0.113.2"}
	var id string
	for i := 0; i < 2; i++ {
		code, out := hs.doAs(t, "POST", "/v1/cases/predict", body, alice)
		if code != http.StatusCreated {
			t.Fatalf("predict %d: %d %v", i, code, out)
		}
		id = out["id"].(string)
	}
	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(body))
	req.Header.Set("X-Client-IP", "203.0.113.1")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"rate_limited"`) {
		t.Fatalf("third predict = %d %s (Retry-After %q)", rec.Code, rec.Body, rec.Header().Get("Retry-After"))
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", body, bob); code != http.StatusCreated {
		t.Errorf("another client was limited: %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", alice); code != http.StatusOK {
		t.Errorf("reads must not be limited: %d", code)
	}
	plan := `{"target":"Thailand"}`
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", plan, alice); code != http.StatusOK {
		t.Errorf("first plan (heavy bucket is separate) = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", plan, alice); code != http.StatusTooManyRequests {
		t.Errorf("second plan = %d, want 429", code)
	}
	adminAlice := map[string]string{"X-Client-IP": "203.0.113.1", "X-Admin-Token": testAdmin}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/"+id+"/plan", plan, adminAlice); code != http.StatusOK {
		t.Errorf("admin was limited: %d", code)
	}
}

func TestGlobalLimitCountsHeavyRequests(t *testing.T) {
	l := newLimiter(Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 7})
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	if ok, _ := l.allow("a", heavy); !ok { // takes 5 of 7
		t.Fatal("first heavy refused")
	}
	if ok, wait := l.allow("b", heavy); ok || wait <= 0 {
		t.Fatalf("second heavy (3 of 5 needed left) = %v, wait %v", ok, wait)
	}
	if ok, _ := l.allow("c", light); !ok {
		t.Fatal("light request refused with 2 left; the refused heavy must not have taken tokens")
	}
	now = now.Add(time.Minute)
	if ok, _ := l.allow("b", heavy); !ok {
		t.Fatal("heavy refused after the bucket refilled")
	}
}

func TestDeleteAndPin(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, a := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	_, b := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	demo, scratch := a["id"].(string), b["id"].(string)

	if code, _ := hs.do(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`); code != http.StatusForbidden {
		t.Errorf("pin without the admin token = %d", code)
	}
	code, out := hs.doAs(t, "PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin)
	if code != http.StatusOK || out["pinned"] != true {
		t.Fatalf("pin = %d %v", code, out)
	}
	if h := out["history"].([]any); h[len(h)-1].(map[string]any)["kind"] != "pinned" {
		t.Errorf("pin not in history: %v", h[len(h)-1])
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	first := list["cases"].([]any)[0].(map[string]any)
	if first["id"] != demo || first["pinned"] != true { // pinned lists first though the scratch case is newer
		t.Errorf("first listed = %v", first)
	}
	if code, _ := hs.do(t, "DELETE", "/v1/cases/"+demo, ""); code != http.StatusForbidden {
		t.Errorf("visitor deleted a pinned case: %d", code)
	}
	if code, _ := hs.do(t, "DELETE", "/v1/cases/"+scratch, ""); code != http.StatusNoContent {
		t.Errorf("delete unpinned = %d", code)
	}
	if code, _ := hs.do(t, "GET", "/v1/cases/"+scratch, ""); code != http.StatusNotFound {
		t.Errorf("deleted case still readable: %d", code)
	}
	if code, _ := hs.do(t, "DELETE", "/v1/cases/"+scratch, ""); code != http.StatusNotFound {
		t.Errorf("second delete = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/cases/"+demo, "", admin); code != http.StatusNoContent {
		t.Errorf("admin delete of a pinned case = %d", code)
	}
	if code, _ := hs.doAs(t, "PUT", "/v1/cases/"+scratch+"/pin", `{}`, admin); code != http.StatusNotFound {
		t.Errorf("pin of a missing case = %d", code)
	}
}

func TestHistoryRecordsUsageAndCost(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	ev := c["history"].([]any)[0].(map[string]any)
	// engine 3000/200 on Argon ($2/$10 per 1M) + router 1000/100 on Flash ($0.75/$3.75)
	if ev["input_tokens"] != 4000.0 || ev["output_tokens"] != 300.0 || ev["cost_usd"] != 0.009125 {
		t.Errorf("predicted usage = %v", ev)
	}
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	h := got["history"].([]any)
	plan := h[len(h)-1].(map[string]any)
	// engine 30000/2000 + verifier 10000/1000, both Argon: 0.08 + 0.03
	if plan["kind"] != "plan_searched" || plan["input_tokens"] != 40000.0 || plan["cost_usd"] != 0.11 || plan["latency_ms"] != 12500.0 {
		t.Errorf("plan usage = %v", plan)
	}
}

func TestRenameCase(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	if c["display_name"] != c["product"].(map[string]any)["name"] {
		t.Errorf("default display_name = %v", c["display_name"])
	}
	code, row := hs.do(t, "PATCH", "/v1/cases/"+id, `{"display_name":"  Heat gun   — Q3 sourcing  "}`)
	if code != http.StatusOK || row["display_name"] != "Heat gun — Q3 sourcing" || row["id"] != id || row["product_name"] == row["display_name"] {
		t.Fatalf("rename = %d %v", code, row)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	h := got["history"].([]any)
	if e := h[len(h)-1].(map[string]any); e["kind"] != "renamed" || e["cost_usd"] != nil || got["updated_at"] != c["updated_at"] {
		t.Errorf("history %v, updated_at %v -> %v", e, c["updated_at"], got["updated_at"])
	}
	for _, bad := range []string{`{"display_name":"   "}`, `{}`, `{"display_name":"` + strings.Repeat("x", 201) + `"}`} {
		if code, _ := hs.do(t, "PATCH", "/v1/cases/"+id, bad); code != http.StatusBadRequest {
			t.Errorf("%s = %d", bad[:20], code)
		}
	}
	hs.doAs(t, "PUT", "/v1/cases/"+id+"/pin", `{"pinned":true}`, admin)
	if code, _ := hs.do(t, "PATCH", "/v1/cases/"+id, `{"display_name":"x"}`); code != http.StatusForbidden {
		t.Errorf("rename pinned = %d", code)
	}
	if code, _ := hs.do(t, "PATCH", "/v1/cases/case_missing", `{"display_name":"x"}`); code != http.StatusNotFound {
		t.Errorf("rename missing = %d", code)
	}
}

type textileEngine struct{ conf float64 }

func (e textileEngine) Predict(context.Context, domain.Case) (engine.Result, error) {
	return engine.Result{Prediction: domain.Prediction{Origin: "Vietnam", Confidence: e.conf, Precedents: []string{"N350001"},
		Trace: domain.Trace{ModelID: "gemini-4-argon", PromptVersion: engine.TextileVersion}}}, nil
}

func TestTextileRegime(t *testing.T) {
	gaiter := `{"date":"2026-03-01","facts":"Knit polyester neck gaiters, knit in Vietnam from Chinese yarn."}`
	// Without a textile engine: out of scope, as before.
	hs := newHarness(t, "textile_102_21")
	_, out := hs.do(t, "POST", "/v1/cases/predict", gaiter)
	if r := out["triage"].(map[string]any)["reason"].(string); !strings.HasPrefix(r, "out of scope") {
		t.Errorf("no textile engine: reason %q", r)
	}
	for _, tt := range []struct {
		fast  float64
		route string
	}{{0, "needs_review"}, {0.9, "auto"}, {0.99, "needs_review"}} {
		hs := newHarnessWith(t, "textile_102_21", func(d *Deps) { d.Textile, d.TextileFastPath = textileEngine{conf: 0.97}, tt.fast })
		code, out := hs.do(t, "POST", "/v1/cases/predict", gaiter)
		if code != http.StatusCreated {
			t.Fatalf("predict = %d %v", code, out)
		}
		p, tr := out["prediction"].(map[string]any), out["triage"].(map[string]any)
		if p["origin_country"] != "Vietnam" || tr["route"] != tt.route || !strings.HasPrefix(tr["reason"].(string), triage.TextilePrefix) || out["clarify"] != nil {
			t.Errorf("fast %.2f: prediction %v triage %v clarify %v", tt.fast, p["origin_country"], tr, out["clarify"])
		}
		if len(hs.clar.facts) != 0 {
			t.Errorf("clarify ran on a textile case")
		}
	}
}

func TestExportOnPinnedCaseDoesNotWriteForVisitors(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.doAs(t, "PUT", "/v1/cases/"+id+"/pin", `{"pinned":true}`, admin)
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", "")
	if code != http.StatusOK || out["content"] == "" {
		t.Fatalf("export = %d %v", code, out)
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if len(after["history"].([]any)) != len(before["history"].([]any)) || after["updated_at"] != before["updated_at"] || after["step"] == "export" {
		t.Errorf("a visitor's export wrote to a pinned case: history %d -> %d, step %v", len(before["history"].([]any)), len(after["history"].([]any)), after["step"])
	}
	hs.doAs(t, "POST", "/v1/cases/"+id+"/export", "", admin)
	_, admin2 := hs.do(t, "GET", "/v1/cases/"+id, "")
	if len(admin2["history"].([]any)) != len(before["history"].([]any))+1 {
		t.Error("the admin's export was not recorded")
	}
}

func TestCaseShowsPricedEconomics(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if c["economics_priced"] != nil {
		t.Errorf("no economics yet, got %v", c["economics_priced"])
	}
	id := c["id"].(string)
	hs.do(t, "PUT", "/v1/cases/"+id+"/economics", `{"illustrative":true,"annual_import_value_usd":2000000}`)
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	e := got["economics_priced"].(map[string]any)
	if r := e["duty_rates"].(map[string]any); r["China"] != 0.417 || len(e["duty_rates_from_table"].([]any)) != 4 {
		t.Errorf("economics_priced = %v", e)
	}
	if r, _ := got["economics"].(map[string]any)["duty_rates"].(map[string]any); len(r) != 0 {
		t.Errorf("table rates leaked into the stored economics: %v", r)
	}
}
