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
)

// Family questions (#770, D810-D815): deterministic, no model call, and the
// answers never change the case's facts, product or parts list.

func seedFamilyCase(t *testing.T, mem *store.Memory, id, family string, p *domain.Product) store.Case {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute)
	c := store.Case{ID: id, Date: "2026-11-20", Facts: "A product described for the test.\n", Family: family, Product: p, CreatedAt: now, UpdatedAt: now,
		Prediction: domain.Prediction{Origin: "Mexico", Reasoning: "Assembled in Mexico.", Trace: domain.Trace{Provider: "fake", ModelID: "fake-model", PromptVersion: "p1"}},
		Triage:     domain.Triage{Route: domain.RouteNeedsReview, Reason: "test"},
		History:    []store.Event{{At: now, Kind: "predicted", Summary: "origin Mexico"}}}
	if err := mem.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func packProduct() *domain.Product {
	return &domain.Product{Name: "Lithium battery pack for e-bikes", Plants: []domain.Plant{{ID: "mx", Country: "Mexico"}},
		Components: []domain.Component{{Name: "18650 cells", Group: "battery_cells"}, {Name: "BMS board", Group: "pcba_manufacturing"}, {Name: "case", MadeIn: "Mexico", Group: "housing_cables_parts"}},
		Operations: []domain.Operation{{Name: "assemble the pack", Plant: "mx", Group: "final_assembly"}}}
}

func familyHarness(t *testing.T) (harness, *store.Memory) {
	t.Helper()
	mem := store.NewMemory()
	return newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases = mem }), mem
}

func intakeOf(t *testing.T, v any) map[string]any {
	t.Helper()
	in, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("no family_intake: %v", v)
	}
	return in
}

func questionStates(in map[string]any) map[string]string {
	out := map[string]string{}
	for _, x := range asList(in["questions"]) {
		q := x.(map[string]any)
		out[q["id"].(string)] = q["state"].(string)
	}
	return out
}

// A battery pack: the case asks where the cells were made, naming the
// document; the answer is saved beside the case and read back; the product,
// facts and prediction are untouched; the list marks the answers.
func TestFamilyFactsBatteryPackEndToEnd(t *testing.T) {
	hs, mem := familyHarness(t)
	c := seedFamilyCase(t, mem, "case_fam_pack", "power_supplies_chargers", packProduct())

	_, cv := hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	in := intakeOf(t, cv["family_intake"])
	if in["status"] != "missing_information" || !reflect.DeepEqual(questionStates(in), map[string]string{"cells_made": "missing", "pack_work": "missing", "bms_made": "missing"}) {
		t.Fatalf("intake: %v", in)
	}
	q := asList(in["questions"])[0].(map[string]any)
	if !strings.Contains(q["evidence"].(string), "certificate of origin") || len(asList(q["confers_origin"])) == 0 {
		t.Errorf("question: %v", q)
	}

	path := "/v1/cases/" + c.ID + "/family-facts"
	code, out := hs.do(t, "PUT", path, `{"expected_updated_at":"","answers":{"cells_made":{"country":"south korea"},"pack_work":{"yes":false}}}`)
	if code != http.StatusOK {
		t.Fatalf("put: %d %v", code, out)
	}
	ff := out["family_facts"].(map[string]any)
	if ff["source"] != domain.SourceUserSupplied || ff["family"] != "power_supplies_chargers" || ff["template_version"] != "2" {
		t.Errorf("saved: %v", ff)
	}
	in = intakeOf(t, out["family_intake"])
	if st := questionStates(in); st["cells_made"] != "answered" || st["pack_work"] != "answered" || st["bms_made"] != "missing" {
		t.Errorf("after save: %v", st)
	}

	got, _ := mem.Get(context.Background(), c.ID)
	if !reflect.DeepEqual(got.Product, c.Product) || got.Facts != c.Facts || got.Prediction.Origin != "Mexico" || len(got.Answers) != 0 || got.OriginalFacts != "" {
		t.Errorf("the case changed: %+v", got)
	}
	if got.FamilyFacts.Answers["cells_made"].Country != "South Korea" {
		t.Errorf("country not normalised: %+v", got.FamilyFacts)
	}
	if e := got.History[len(got.History)-1]; e.Kind != "family_facts" || !strings.Contains(e.Summary, "2 answered, 1 open") {
		t.Errorf("event: %+v", e)
	}

	_, list := hs.do(t, "GET", "/v1/cases", "")
	for _, x := range asList(list["cases"]) {
		if row := x.(map[string]any); row["id"] == c.ID && row["answers_note"] != "added" {
			t.Errorf("list row: %v", row)
		}
	}
	// The evidence report lists them apart from the facts.
	_, rep := hs.do(t, "GET", "/v1/cases/"+c.ID+"/evidence-report", "")
	md, _ := rep["markdown"].(string)
	if !strings.Contains(md, "## Answers to the family questions") || !strings.Contains(md, "Where were the battery cells made? South Korea") {
		t.Errorf("report:\n%s", md)
	}
	// Clearing.
	if code, out = hs.do(t, "PUT", path, `{}`); code != 200 || out["family_facts"] != nil {
		t.Errorf("clear: %d %v", code, out)
	}
}

func TestFamilyFactsRefusals(t *testing.T) {
	hs, mem := familyHarness(t)
	c := seedFamilyCase(t, mem, "case_fam_bad", "power_supplies_chargers", packProduct())
	path := "/v1/cases/" + c.ID + "/family-facts"
	for name, body := range map[string]string{
		"unknown question": `{"answers":{"lens_ground":{"country":"China"}}}`,
		"yes on a country": `{"answers":{"cells_made":{"yes":true}}}`,
		"bad country":      `{"answers":{"cells_made":{"country":"<b>x</b>"}}}`,
		"country on yes":   `{"answers":{"pack_work":{"country":"China"}}}`,
		"unknown":          `{"answers":{"cells_made":{"country":"Unknown"}}}`,
		"tbd":              `{"answers":{"cells_made":{"country":"TBD"}}}`,
		"not a country":    `{"answers":{"cells_made":{"country":"Mars"}}}`,
		"unknown field":    `{"answers":{},"extra":1}`,
	} {
		if code, _ := hs.do(t, "PUT", path, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	// Not modelled: no answer is taken.
	drill := seedFamilyCase(t, mem, "case_fam_drill", "power_supplies_chargers", &domain.Product{Name: "Cordless drill with battery", Components: []domain.Component{{Name: "motor"}}})
	_, cv := hs.do(t, "GET", "/v1/cases/"+drill.ID, "")
	in := intakeOf(t, cv["family_intake"])
	if in["status"] != "not_modelled" || in["not_modelled"].(map[string]any)["code"] != "guard" || len(asList(in["questions"])) != 0 {
		t.Errorf("drill: %v", in)
	}
	if code, out := hs.do(t, "PUT", "/v1/cases/"+drill.ID+"/family-facts", `{"answers":{"cells_made":{"country":"China"}}}`); code != 400 || !strings.Contains(out["message"].(string), "no family questions") {
		t.Errorf("drill save: %d %v", code, out)
	}
	// A case of a family with no questions carries no family_intake (as before).
	toy := seedFamilyCase(t, mem, "case_fam_toy", "toys_build_kits", &domain.Product{Name: "Building blocks", Components: []domain.Component{{Name: "bricks"}}})
	if _, cv := hs.do(t, "GET", "/v1/cases/"+toy.ID, ""); cv["family_intake"] != nil {
		t.Errorf("toy: %v", cv["family_intake"])
	}
	if code, _ := hs.do(t, "PUT", "/v1/cases/"+toy.ID+"/family-facts", `{"answers":{"frame_made":{"country":"China"}}}`); code != 400 {
		t.Errorf("toy save: %d", code)
	}
	// Pinned demo: read only.
	c.Pinned = true
	if err := mem.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if code, _ := hs.do(t, "PUT", path, `{"answers":{"cells_made":{"country":"China"}}}`); code != http.StatusForbidden {
		t.Errorf("pinned: %d", code)
	}
}

func TestFamilyFactsPreconditionAndStale(t *testing.T) {
	hs, mem := familyHarness(t)
	c := seedFamilyCase(t, mem, "case_fam_pre", "medical_devices", &domain.Product{Name: "Surgical drill bits", Components: []domain.Component{{Name: "steel blank"}}})
	path := "/v1/cases/" + c.ID + "/family-facts"
	if code, _ := hs.do(t, "PUT", path, `{"expected_updated_at":"2020-01-01T00:00:00Z","answers":{"sterile_only":{"yes":true}}}`); code != 409 {
		t.Errorf("precondition on empty: %d", code)
	}
	code, out := hs.do(t, "PUT", path, `{"expected_updated_at":"","answers":{"blanks_made":{"country":"China"},"sterile_only":{"yes":true}}}`)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	at := out["family_facts"].(map[string]any)["updated_at"].(string)
	if code, _ := hs.do(t, "PUT", path, `{"expected_updated_at":"","answers":{"sterile_only":{"yes":false}}}`); code != 409 {
		t.Errorf("second writer: %d", code)
	}
	if code, _ := hs.do(t, "PUT", path, mustJSON(map[string]any{"expected_updated_at": at, "answers": map[string]any{"sterile_only": map[string]bool{"yes": false}}})); code != 200 {
		t.Errorf("matching precondition: %d", code)
	}
	if _, err := mem.Update(context.Background(), c.ID, func(x *store.Case) error {
		x.Record(store.Event{At: time.Now().UTC().Add(time.Second), Kind: "bom_updated", Summary: "new BOM"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, cv := hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	in := intakeOf(t, cv["family_intake"])
	if in["stale"] != true || questionStates(in)["sterile_only"] != "missing" || cv["family_facts"].(map[string]any)["stale"] != true {
		t.Errorf("stale: %v %v", in, cv["family_facts"])
	}
	if got, _ := mem.Get(context.Background(), c.ID); got.FamilyFacts.Stale {
		t.Error("stale was stored")
	}
}

// Saving answers on an EU case re-runs the stored check (deterministic) and
// keeps the first answer (D815); the list says so.
func TestOriginFactsRecheckStoredPrediction(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_restore", "eu", radioProduct())
	path := "/v1/cases/" + c.ID + "/origin-facts/eu"
	body := `{"answers":{"ch85.r":false},"materials":[{"index":0,"name":"tuner","part_hts":"8529.90","value_share":0.7},{"index":1,"name":"case","part_hts":"3926.90","value_share":0.2}]}`
	code, out := hs.do(t, "PUT", path, body)
	if code != 200 || out["prediction"].(map[string]any)["origin_country"] != "China" {
		t.Fatalf("%d %v", code, out)
	}
	got, _ := mem.Get(context.Background(), c.ID)
	if got.Prediction.Origin != "China" || got.Prediction.Trace.Provider != "rules" || got.PredictionBeforeAnswers == nil || got.PredictionBeforeAnswers.Origin != "Malaysia" {
		t.Fatalf("stored: %+v / %+v", got.Prediction, got.PredictionBeforeAnswers)
	}
	if e := got.History[len(got.History)-1]; e.Kind != "rechecked" || !strings.Contains(e.Summary, "was Malaysia, now China") {
		t.Errorf("event: %+v", e)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	for _, x := range asList(list["cases"]) {
		if row := x.(map[string]any); row["id"] == c.ID && (row["answers_note"] != "rechecked" || row["predicted_origin"] != "China") {
			t.Errorf("list row: %v", row)
		}
	}
	// Clearing re-runs on the codes alone; the first answer stays kept.
	if code, _ = hs.do(t, "PUT", path, `{}`); code != 200 {
		t.Fatal(code)
	}
	got, _ = mem.Get(context.Background(), c.ID)
	if got.Prediction.Origin == "China" || got.PredictionBeforeAnswers.Origin != "Malaysia" || answersNote(got) != "" {
		t.Errorf("after clear: %+v %q", got.Prediction, answersNote(got))
	}
	// A U.S. case's answers to the EU check do not touch its stored answer.
	us := seedCase(t, mem, "case_us_eu_facts", "us", radioProduct())
	if code, out := hs.do(t, "PUT", "/v1/cases/"+us.ID+"/origin-facts/eu", body); code != 200 || out["prediction"] != nil {
		t.Errorf("us: %d %v", code, out)
	}
	if got, _ := mem.Get(context.Background(), us.ID); got.Prediction.Origin != "Malaysia" || got.PredictionBeforeAnswers != nil || answersNote(got) != "added" {
		t.Errorf("us stored: %+v %q", got.Prediction, answersNote(got))
	}
}

// save_case_answers runs the same routes: validation, precondition and the
// re-check, over MCP.
func TestMCPSaveCaseAnswers(t *testing.T) {
	hs, mem := originHarness(t)
	fam := seedFamilyCase(t, mem, "case_mcp_pack", "power_supplies_chargers", packProduct())
	eu := seedCase(t, mem, "case_mcp_eu", "eu", radioProduct())
	cs := mcpClient(t, hs, nil)

	res, out := callTool(t, cs, "save_case_answers", map[string]any{"case_id": fam.ID, "check": "family", "expected_updated_at": "",
		"family_answers": map[string]any{"cells_made": map[string]any{"country": "China"}}})
	if res.IsError || !strings.Contains(text(res), "Still missing") || !strings.Contains(text(res), "settled by") {
		t.Fatalf("family: %s", text(res))
	}
	if out["family_facts"].(map[string]any)["answers"].(map[string]any)["cells_made"] == nil {
		t.Errorf("family out: %v", out)
	}
	// The same precondition as REST: the empty state is gone now.
	if res, _ = callTool(t, cs, "save_case_answers", map[string]any{"case_id": fam.ID, "check": "family", "expected_updated_at": "",
		"family_answers": map[string]any{"cells_made": map[string]any{"country": "Vietnam"}}}); !res.IsError || !strings.Contains(text(res), "changed since") {
		t.Errorf("precondition: %s", text(res))
	}
	// The same validation: a question the case does not ask.
	if res, _ = callTool(t, cs, "save_case_answers", map[string]any{"case_id": fam.ID, "check": "family",
		"family_answers": map[string]any{"lens_ground": map[string]any{"country": "China"}}}); !res.IsError {
		t.Errorf("unknown question accepted: %s", text(res))
	}
	if res, _ = callTool(t, cs, "save_case_answers", map[string]any{"case_id": fam.ID, "check": "family", "row": "none"}); !res.IsError {
		t.Errorf("eu field on family accepted: %s", text(res))
	}
	if got, _ := mem.Get(context.Background(), fam.ID); got.FamilyFacts.Answers["cells_made"].Country != "China" || !reflect.DeepEqual(got.Product, fam.Product) {
		t.Errorf("stored: %+v", got.FamilyFacts)
	}
	if res, _ = callTool(t, cs, "get_case", map[string]any{"case_id": fam.ID}); !strings.Contains(text(res), "2 family question(s) open") || !strings.Contains(text(res), "save_case_answers") {
		t.Errorf("get_case: %s", text(res))
	}
	if res, _ = callTool(t, cs, "save_case_answers", map[string]any{"case_id": fam.ID, "check": "family"}); res.IsError || !strings.Contains(text(res), "Cleared") || strings.Contains(text(res), "not re-run") {
		t.Errorf("clear: %s", text(res))
	}

	res, out = callTool(t, cs, "save_case_answers", map[string]any{"case_id": eu.ID, "check": "eu", "answers": map[string]any{"ch85.r": false},
		"materials": []any{map[string]any{"index": 0, "name": "tuner", "part_hts": "8529.90", "value_share": 0.7}, map[string]any{"index": 1, "name": "case", "part_hts": "3926.90", "value_share": 0.2}}})
	if res.IsError || !strings.Contains(text(res), "China") || !strings.Contains(text(res), "stored answer was re-run") || out["prediction"] == nil {
		t.Fatalf("eu: %s", text(res))
	}
	if res, _ = callTool(t, cs, "save_case_answers", map[string]any{"case_id": eu.ID, "check": "eu", "answers": map[string]any{"nope": true}}); !res.IsError {
		t.Errorf("unknown eu key accepted: %s", text(res))
	}
}

// Saves and reads at once (run with -race): no model call, ever.
func TestFamilyFactsConcurrentPutAndGet(t *testing.T) {
	hs, mem := familyHarness(t)
	c := seedFamilyCase(t, mem, "case_fam_race", "power_supplies_chargers", packProduct())
	hs.engine.mu.Lock()
	before := hs.engine.calls
	hs.engine.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			country := []string{"China", "South Korea"}[i%2]
			hs.do(t, "PUT", "/v1/cases/"+c.ID+"/family-facts", mustJSON(map[string]any{"answers": map[string]any{"cells_made": map[string]string{"country": country}}}))
		}
	}()
	for i := 0; i < 20; i++ {
		if code, _ := hs.do(t, "GET", "/v1/cases/"+c.ID, ""); code != 200 {
			t.Errorf("get %d", code)
		}
		hs.do(t, "GET", "/v1/cases/"+c.ID+"/evidence-report", "")
		hs.do(t, "GET", "/v1/cases", "")
	}
	<-done
	hs.engine.mu.Lock()
	calls := hs.engine.calls
	hs.engine.mu.Unlock()
	if calls != before {
		t.Errorf("engine calls %d -> %d: family answers called the model", before, calls)
	}
	if got, _ := mem.Get(context.Background(), c.ID); !reflect.DeepEqual(got.Product, c.Product) || got.Facts != c.Facts {
		t.Error("the case changed")
	}
}

// The kept answer is shown only while the newest stored prediction is the
// re-check; a new prediction (a BOM update) makes it history, and the next
// re-check keeps the newer one (D818).
func TestPredictionBeforeAnswersFollowsNewPredictions(t *testing.T) {
	hs, mem := originHarness(t)
	c := seedCase(t, mem, "case_eu_pba", "eu", radioProduct())
	path := "/v1/cases/" + c.ID + "/origin-facts/eu"
	if code, _ := hs.do(t, "PUT", path, `{"answers":{"ch85.r":false}}`); code != 200 {
		t.Fatal(code)
	}
	_, cv := hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	if cv["prediction_before_answers"].(map[string]any)["origin_country"] != "Malaysia" {
		t.Fatalf("kept: %v", cv["prediction_before_answers"])
	}
	// A BOM update stores a new prediction.
	if _, err := mem.Update(context.Background(), c.ID, func(x *store.Case) error {
		x.Prediction = domain.Prediction{Origin: "Thailand", Trace: domain.Trace{Provider: "rules"}}
		x.Record(store.Event{At: time.Now().UTC().Add(time.Second), Kind: "bom_updated", Summary: "new BOM"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, cv = hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	if cv["prediction_before_answers"] != nil {
		t.Errorf("shown after a new prediction: %v", cv["prediction_before_answers"])
	}
	got, _ := mem.Get(context.Background(), c.ID)
	if n := answersNote(got); n != "" {
		t.Errorf("stale answers noted %q", n)
	}
	time.Sleep(1100 * time.Millisecond) // after the BOM event
	if code, _ := hs.do(t, "PUT", path, `{"answers":{"ch85.r":false}}`); code != 200 {
		t.Fatal(code)
	}
	_, cv = hs.do(t, "GET", "/v1/cases/"+c.ID, "")
	if cv["prediction_before_answers"].(map[string]any)["origin_country"] != "Thailand" {
		t.Errorf("kept after re-answer: %v", cv["prediction_before_answers"])
	}
	// The stored copy shares nothing with the prediction.
	got, _ = mem.Get(context.Background(), c.ID)
	got.PredictionBeforeAnswers.KeyFactors = append(got.PredictionBeforeAnswers.KeyFactors, "x")
	if again, _ := mem.Get(context.Background(), c.ID); len(again.PredictionBeforeAnswers.KeyFactors) != len(got.PredictionBeforeAnswers.KeyFactors)-1 {
		t.Error("write-through on the kept prediction")
	}
}

// Cleared or stale family answers say nothing on the case list.
func TestAnswersNoteIgnoresClearedAndStale(t *testing.T) {
	hs, mem := familyHarness(t)
	c := seedFamilyCase(t, mem, "case_fam_note", "power_supplies_chargers", packProduct())
	path := "/v1/cases/" + c.ID + "/family-facts"
	hs.do(t, "PUT", path, `{"answers":{"cells_made":{"country":"China"}}}`)
	got, _ := mem.Get(context.Background(), c.ID)
	if answersNote(got) != "added" {
		t.Fatalf("note %q", answersNote(got))
	}
	hs.do(t, "PUT", path, `{}`)
	if got, _ = mem.Get(context.Background(), c.ID); answersNote(got) != "" {
		t.Errorf("cleared: %q", answersNote(got))
	}
	hs.do(t, "PUT", path, `{"answers":{"cells_made":{"country":"China"}}}`)
	if _, err := mem.Update(context.Background(), c.ID, func(x *store.Case) error {
		x.Record(store.Event{At: time.Now().UTC().Add(time.Second), Kind: "bom_updated", Summary: "new BOM"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ = mem.Get(context.Background(), c.ID); answersNote(got) != "" {
		t.Errorf("stale: %q", answersNote(got))
	}
}
