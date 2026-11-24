package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
)

// Synthetic products run through the committed EU and UK tables (#770,
// D800-D802). Nothing here injects a rule table.

func radioProduct() *domain.Product {
	return &domain.Product{Name: "Radio", HTS: "8527.13",
		Plants:     []domain.Plant{{ID: "my", Country: "Malaysia"}},
		Components: []domain.Component{{Name: "tuner", HTS: "8529.90", MadeIn: "China", Plant: "my"}, {Name: "case", HTS: "3926.90", MadeIn: "Malaysia", Plant: "my"}},
		Operations: []domain.Operation{{Name: "assemble", Plant: "my", Group: "final_assembly"}}}
}

func seedCase(t *testing.T, mem *store.Memory, id, market string, p *domain.Product) store.Case {
	t.Helper()
	now := time.Now().UTC()
	c := store.Case{ID: id, Date: "2026-11-20", Facts: "A product described for the test.\n", Product: p,
		Jurisdiction: &domain.Jurisdiction{Code: market, Source: "chosen"}, CreatedAt: now, UpdatedAt: now,
		Prediction: domain.Prediction{Origin: "Malaysia", Reasoning: "Assembled in Malaysia. The tuner is Chinese. More.", Trace: domain.Trace{ModelID: "fake-model", PromptVersion: "p1"}},
		Triage:     domain.Triage{Route: domain.RouteNeedsReview, Reason: "test"}}
	if err := mem.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func originHarness(t *testing.T) (harness, *store.Memory) {
	t.Helper()
	eu, uk := originRules(t)
	mem := store.NewMemory()
	ca, err := tariffshift.CanadaRules()
	if err != nil {
		t.Fatal(err)
	}
	us, err := tariffshift.Default()
	if err != nil {
		t.Fatal(err)
	}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.EUOrigin, d.UKOrigin, d.Cases, d.CanadaMarking, d.RuleCheck = eu, uk, mem, ca, us
	})
	return hs, mem
}

func checkOf(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	ch, ok := out["check"].(map[string]any)
	if !ok {
		t.Fatalf("no check: %v", out)
	}
	return ch["result"].(map[string]any)
}

func needKeys(res map[string]any) []string {
	var out []string
	for _, x := range asList(res["needs"]) {
		n := x.(map[string]any)
		out = append(out, n["kind"].(string)+":"+n["key"].(string))
	}
	return out
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// The EU radio: the intake asks for the tuner's value share (10% tolerance),
// then the chapter rule for blanks, then the case's value share (the
// residual rule), and decides. The product never changes.
func TestOriginFactsEUValueShareEndToEnd(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_radio", "eu", radioProduct())
	path := "/v1/cases/" + c.ID + "/origin-facts/eu"

	code, out := hs.do(t, "POST", "/v1/rules/eu-origin", mustJSON(map[string]any{"product": c.Product}))
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	res := out["result"].(map[string]any)
	if res["status"] != "undetermined" || !reflect.DeepEqual(needKeys(res), []string{"share:value_share"}) {
		t.Fatalf("first need: %v", res)
	}
	if ev := asList(res["needs"])[0].(map[string]any)["evidence"]; !strings.Contains(ev.(string), "bill of materials with values") {
		t.Errorf("evidence: %v", ev)
	}

	code, out = hs.do(t, "PUT", path, `{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7}]}`)
	if code != http.StatusOK {
		t.Fatalf("put 1: %d %v", code, out)
	}
	if res = checkOf(t, out); !reflect.DeepEqual(needKeys(res), []string{"fact:ch85.r"}) {
		t.Fatalf("second need: %v", res)
	}
	code, out = hs.do(t, "PUT", path, `{"answers":{"ch85.r":false},"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7}]}`)
	if res = checkOf(t, out); code != http.StatusOK || !reflect.DeepEqual(needKeys(res), []string{"share:value_share"}) {
		t.Fatalf("third need: %d %v", code, res)
	}
	code, out = hs.do(t, "PUT", path, `{"answers":{"ch85.r":false},"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7},{"index":1,"name":"case","part_hts":"3926.90","value_share":0.2}]}`)
	if res = checkOf(t, out); code != http.StatusOK || res["status"] != "decided" || res["origin"] != "China" {
		t.Fatalf("decided: %d %v", code, res)
	}
	sup := out["check"].(map[string]any)["supplied"].(map[string]any)
	if sup["source"] != domain.SourceUserSupplied || sup["updated_at"] == "" {
		t.Errorf("supplied: %v", sup)
	}

	got, err := mem.Get(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Product, c.Product) || got.Facts != c.Facts {
		t.Errorf("the product or facts changed: %+v", got.Product)
	}
	f := got.OriginFacts["eu"]
	if f == nil || f.Source != domain.SourceUserSupplied || f.UpdatedAt.IsZero() || len(f.Materials) != 2 || f.Answers["ch85.r"] {
		t.Fatalf("stored: %+v", f)
	}
	if e := got.History[len(got.History)-1]; e.Kind != "origin_facts" || !strings.Contains(e.Summary, "decided") {
		t.Errorf("event %+v", e)
	}
	// The case answer carries them; the stateless route reads them.
	_, cv := hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	if cv["origin_facts"].(map[string]any)["eu"] == nil {
		t.Errorf("case answer: %v", cv["origin_facts"])
	}
	body := mustJSON(map[string]any{"product": c.Product, "supplied": got.OriginFacts["eu"]})
	if _, out = hs.do(t, "POST", "/v1/rules/eu-origin", body); out["result"].(map[string]any)["origin"] != "China" {
		t.Errorf("stateless with supplied: %v", out["result"])
	}
	// Clearing.
	code, out = hs.do(t, "PUT", path, `{}`)
	if code != http.StatusOK || len(out["origin_facts"].(map[string]any)) != 0 {
		t.Fatalf("clear: %d %v", code, out)
	}
	if got, _ = mem.Get(context.Background(), c.ID); got.OriginFacts != nil {
		t.Errorf("not cleared: %+v", got.OriginFacts)
	}
}

// The UK trainers: a process fact (not only assembly of uppers), answered
// yes, decides; a split heading takes a row; a laptop gets no question.
func TestOriginFactsUKProcessRowAndJudgement(t *testing.T) {
	hs, mem := originHarness(t)
	trainers := &domain.Product{Name: "Trainers", HTS: "6404.11", Plants: []domain.Plant{{ID: "kh", Country: "Cambodia"}},
		Components: []domain.Component{{Name: "uppers", HTS: "6406.10", MadeIn: "Vietnam", Plant: "kh"}},
		Operations: []domain.Operation{{Name: "last and sole", Plant: "kh", Group: "final_assembly"}}}
	c := seedCase(t, mem, "case_uk_trainers", "uk", trainers)
	_, out := hs.do(t, "POST", "/v1/rules/uk-origin", mustJSON(map[string]any{"product": trainers}))
	res := out["result"].(map[string]any)
	if !reflect.DeepEqual(needKeys(res), []string{"fact:6404.1.c"}) {
		t.Fatalf("need: %v", res)
	}
	if ev := asList(res["needs"])[0].(map[string]any)["evidence"].(string); !strings.Contains(ev, "production record") {
		t.Errorf("evidence %q", ev)
	}
	code, out := hs.do(t, "PUT", "/v1/cases/"+c.ID+"/origin-facts/uk", `{"answers":{"6404.1.c":true}}`)
	if res = checkOf(t, out); code != http.StatusOK || res["status"] != "decided" || res["origin"] != "Cambodia" {
		t.Fatalf("decided: %d %v", code, res)
	}

	shirt := &domain.Product{Name: "Shirt", HTS: "6205.20", Plants: []domain.Plant{{ID: "bd", Country: "Bangladesh"}},
		Components: []domain.Component{{Name: "fabric", HTS: "5208.21", MadeIn: "China", Plant: "bd"}}}
	s := seedCase(t, mem, "case_uk_shirt", "uk", shirt)
	_, out = hs.do(t, "POST", "/v1/rules/uk-origin", mustJSON(map[string]any{"product": shirt}))
	if res = out["result"].(map[string]any); !reflect.DeepEqual(needKeys(res), []string{"row:row"}) {
		t.Fatalf("row need: %v", res)
	}
	code, out = hs.do(t, "PUT", "/v1/cases/"+s.ID+"/origin-facts/uk", `{"row":"6205xa"}`)
	if res = checkOf(t, out); code != http.StatusOK || res["status"] == "undetermined" && needKeys(res)[0] == "row:row" {
		t.Fatalf("row answered: %d %v", code, res)
	}

	laptop := &domain.Product{Name: "Laptop", HTS: "8471.30", Plants: []domain.Plant{{ID: "vn", Country: "Vietnam"}},
		Components: []domain.Component{{Name: "board", HTS: "8473.30", MadeIn: "China", Plant: "vn"}}}
	_, out = hs.do(t, "POST", "/v1/rules/uk-origin", mustJSON(map[string]any{"product": laptop}))
	if res = out["result"].(map[string]any); res["status"] != "needs_judgement" || len(asList(res["needs"])) != 0 {
		t.Fatalf("laptop: %v", res)
	}
}

func TestOriginFactsRefusesWhatTheRulesDoNotAsk(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_bad", "eu", radioProduct())
	path := "/v1/cases/" + c.ID + "/origin-facts/eu"
	for _, bad := range []string{
		`{"answers":{"simple_assembly":true}}`,
		`{"row":"9999"}`,
		`{"good_hts":"8528.72"}`,
		`{"materials":[{"index":5,"name":"x","value_share":0.1}]}`,
		`{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":1.5}]}`,
		`{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7},{"index":1,"name":"case","part_hts":"3926.90","value_share":0.5}]}`,
		`{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","origin":"Japan"}]}`,
		`{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","hts":"8471.30"}]}`,
		`{"materials":[{"index":0,"name":"tuner"},{"index":0,"name":"tuner"}]}`,
		`{"unknown":1}`,
	} {
		if code, out := hs.do(t, "PUT", path, bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d %v", bad, code, out)
		}
	}
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+c.ID+"/origin-facts/ca", `{}`); code != http.StatusBadRequest {
		t.Errorf("market ca: %d", code)
	}
	if got, _ := mem.Get(context.Background(), c.ID); got.OriginFacts != nil {
		t.Errorf("a refused request stored facts: %+v", got.OriginFacts)
	}
	// A parts change makes earlier facts stale: they are not read.
	if code, _ := hs.do(t, "PUT", path, `{"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7}]}`); code != http.StatusOK {
		t.Fatal(code)
	}
	got, _ := mem.Get(context.Background(), c.ID)
	p := *got.Product
	p.Components = []domain.Component{{Name: "speaker", HTS: "8518.29", MadeIn: "China", Plant: "my"}, p.Components[1]}
	_, out := hs.do(t, "POST", "/v1/rules/eu-origin", mustJSON(map[string]any{"product": p, "supplied": got.OriginFacts["eu"]}))
	if st := asList(out["stale_supplied"]); len(st) != 1 {
		t.Errorf("stale: %v", out["stale_supplied"])
	}
	// supplied without a product is refused.
	if code, _ := hs.do(t, "POST", "/v1/rules/eu-origin", `{"good_hts":"8527","supplied":{"row":"none"}}`); code != http.StatusBadRequest {
		t.Errorf("supplied without product: %d", code)
	}
}

func TestOriginFactsPinnedDemoIsReadOnly(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_demo", "eu", radioProduct())
	c.Pinned = true
	if err := mem.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+c.ID+"/origin-facts/eu", `{"answers":{"ch85.r":false}}`); code != http.StatusForbidden {
		t.Errorf("pinned demo: %d", code)
	}
}
