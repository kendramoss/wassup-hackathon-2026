package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// D470: every plan a response returns carries plan_id (its move key), on
// the plan answer and when the case is read; export and a broker review
// link take it, alone or equal to move_key; it is never stored.
func TestPlanIDOnPlansAndInExport(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	code, pl := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	if code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, pl)
	}
	check := func(where string, plan map[string]any) string {
		t.Helper()
		plans := plan["plans"].([]any)
		if len(plans) == 0 {
			t.Fatalf("%s: no shown plan", where)
		}
		for _, list := range []any{plan["plans"], plan["rejected"]} {
			for _, x := range list.([]any) {
				p := x.(map[string]any)
				var parts []string
				for _, m := range p["move"].([]any) {
					mm := m.(map[string]any)
					parts = append(parts, mm["group"].(string)+"@"+mm["to_plant"].(string))
				}
				if want := strings.Join(parts, "+"); want != "" && p["plan_id"] != want {
					t.Errorf("%s: plan_id %v, move key %s", where, p["plan_id"], want)
				}
			}
		}
		return plans[0].(map[string]any)["plan_id"].(string)
	}
	first := check("plan answer", pl["plan"].(map[string]any))
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	if again := check("case read", got["plan"].(map[string]any)); again != first {
		t.Errorf("plan_id changed between the answer and a read: %s, %s", first, again)
	}

	_, byIndex := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"preview":true}`)
	for _, body := range []string{`{"plan_id":"` + first + `","preview":true}`, `{"plan_id":"` + first + `","move_key":"` + first + `","preview":true}`} {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", body)
		if code != http.StatusOK || out["content"] != byIndex["content"] {
			t.Errorf("export %s = %d, want plan_index 0's draft", body, code)
		}
	}
	for _, body := range []string{`{"plan_id":"` + first + `","plan_index":0}`, `{"plan_id":"` + first + `","move_key":"pcba@th"}`, `{"plan_id":"nope@xx"}`} {
		if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", body); code != http.StatusBadRequest {
			t.Errorf("export %s = %d %v, want 400", body, code, out)
		}
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":9}`); code != http.StatusBadRequest || !strings.Contains(out["message"].(string), "starts at 0") {
		t.Errorf("out-of-range plan_index = %d %v", code, out)
	}
	code, cmp := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`)
	if code != http.StatusOK {
		t.Fatalf("compare = %d %v", code, cmp)
	}
	for _, x := range cmp["targets"].([]any) {
		if b, ok := x.(map[string]any)["best"].(map[string]any); ok && b["plan_id"] == nil {
			t.Errorf("compare best without plan_id: %v", x)
		}
	}
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/reviews", `{"plan_id":"`+first+`"}`); code != http.StatusCreated && code != http.StatusOK {
		t.Errorf("review link by plan_id = %d %v", code, out)
	}
}
