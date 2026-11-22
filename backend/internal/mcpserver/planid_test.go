package mcpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// D470: plan lines name each plan by plan_id and a 0-based plan_index, with
// no 1-based numbering beside them (D451: a04, a10 drafted plan "1" as
// plan_index 1), and a search that shows no plan says so.
func TestPlanLinesUsePlanIDAndZeroBasedIndex(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/case_x/plan": `{"case_id":"case_x","plan":{"target":"Malaysia","plans":[` +
		`{"plan_id":"pcba@my","move":[{"group":"pcba","to_plant":"my"}],"description":"Move the PCBA to Malaysia","prediction":{"origin_country":"Malaysia"},"verdict":{"supporting_ruling":"N350383","confidence":0.98}},` +
		`{"plan_id":"pcba@my+housing@my","move":[{"group":"pcba","to_plant":"my"},{"group":"housing","to_plant":"my"}],"description":"Move the PCBA and the housing to Malaysia","prediction":{"origin_country":"Malaysia"}}` +
		`],"rejected":[]}}`}}
	cs := connect(t, api, false, nil)
	res, txt, out := call(t, cs, "plan_production_moves", map[string]any{"case_id": "case_x", "target_country": "Malaysia"})
	if res.IsError {
		t.Fatal(txt)
	}
	for _, want := range []string{"- plan_id pcba@my (plan_index 0): Move the PCBA", "- plan_id pcba@my+housing@my (plan_index 1): Move the PCBA and the housing", "pass its plan_id to export_ruling_request_draft"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "\n1. ") || strings.Contains(txt, NoPlanLine) {
		t.Errorf("1-based numbering or the no-plan line in a search with plans:\n%s", txt)
	}
	plans := obj(out["plan"])["plans"].([]any)
	if p := obj(plans[1]); p["plan_index"] != 1.0 || p["plan_id"] != "pcba@my+housing@my" {
		t.Errorf("structured plan 1: %v", p)
	}
}

func TestNoPlanSaysNoMoveIsSupported(t *testing.T) {
	api := &stubAPI{body: map[string]string{"POST /v1/cases/case_x/plan": `{"case_id":"case_x","plan":{"target":"Vietnam","plans":[],"rejected":[` +
		`{"move":[],"description":"No change: keep the current setup","prediction":{"origin_country":"Cambodia"},"rejection_reason":"current setup"}]}}`}}
	cs := connect(t, api, false, nil)
	_, txt, _ := call(t, cs, "plan_production_moves", map[string]any{"case_id": "case_x", "target_country": "Vietnam"})
	for _, want := range []string{"0 precedent-supported plans", NoPlanLine, "without plan_id, plan_index or move_key"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
}

// export and the broker review link forward plan_id; the REST API reads it
// as the move key.
func TestExportAndReviewForwardPlanID(t *testing.T) {
	var got []map[string]any
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		got = append(got, b)
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			_, _ = w.Write([]byte(`{"review":{"draft_label":"plan"},"review_path":"/review#t","token":"t"}`))
			return
		}
		_, _ = w.Write([]byte(`{"case_id":"c","format":"markdown","content":"# Draft"}`))
	})
	cs := connect(t, api, false, nil)
	call(t, cs, "export_ruling_request_draft", map[string]any{"case_id": "c", "plan_id": "pcba@my"})
	call(t, cs, "export_ruling_request_draft", map[string]any{"case_id": "c"})
	call(t, cs, "create_broker_review_link", map[string]any{"case_id": "c", "plan_id": "pcba@my"})
	if len(got) != 3 || got[0]["plan_id"] != "pcba@my" || got[0]["plan_index"] != nil || got[2]["plan_id"] != "pcba@my" {
		t.Fatalf("bodies %v", got)
	}
	if _, ok := got[1]["plan_id"]; ok || got[1]["plan_index"] != nil || got[1]["move_key"] != nil {
		t.Errorf("current setup body %v", got[1])
	}
}

// The tool descriptions say plan_index starts at 0 and that no selector
// drafts the current setup.
func TestExportDescribesZeroBasedIndexAndCurrentSetup(t *testing.T) {
	cs := connect(t, &stubAPI{}, false, nil)
	lt, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range lt.Tools {
		if tl.Name != "export_ruling_request_draft" {
			continue
		}
		b, _ := json.Marshal(tl)
		s := string(b)
		for _, want := range []string{"0-based", "0 is the first plan", "plan_id", "current setup"} {
			if !strings.Contains(s, want) {
				t.Errorf("export description lacks %q", want)
			}
		}
		return
	}
	t.Fatal("export_ruling_request_draft not listed")
}

// D471: suggest_part_hts shows a flagged code with its note and says how
// many codes to check.
func TestPartHTSShowsCheckNotes(t *testing.T) {
	api := &stubAPI{body: map[string]string{
		"POST /v1/products/parts-hts-suggest": `{"parts":[{"index":0,"name":"centre bar","candidates":[{"hts":"1704.90","source":"rulings","check":"Chapter 17 (sugars and sugar confectionery) is unusual for a part of this product family (vehicle_parts); check this code."},{"hts":"8708.99","source":"model"}]}],"flagged":1}`,
		"GET /v1/products/parts-hts-suggest":  `{}`,
	}}
	cs := connectOpts(t, Options{API: api, PartsHTS: true})
	res, txt, _ := call(t, cs, "suggest_part_hts", map[string]any{"product": map[string]any{"name": "CV axle", "components": []any{map[string]any{"name": "centre bar", "made_in": "China"}}}})
	if res.IsError {
		t.Fatal(txt)
	}
	for _, want := range []string{"1704.90 (rulings) [Chapter 17", "check this code.]", "8708.99 (model)", `1 code is marked "check this code"`} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
}
