package httpapi

import (
	"net/http"
	"testing"
)

func TestCaseOwnership(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CaseOwnership = true })
	alice := map[string]string{"X-Visitor-ID": "alice-visitor-id-0001"}
	bob := map[string]string{"X-Visitor-ID": "bob-visitor-id-000002"}
	_, a := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), alice)
	_, b := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), bob)
	_, d := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), admin)
	aid, bid, did := a["id"].(string), b["id"].(string), d["id"].(string)
	hs.doAs(t, "PUT", "/v1/cases/"+did+"/pin", `{"pinned":true}`, admin)
	if _, ok := a["owner"]; ok {
		t.Error("the owner hash is returned to the client")
	}

	ids := func(h map[string]string) map[string]bool {
		_, out := hs.doAs(t, "GET", "/v1/cases", "", h)
		m := map[string]bool{}
		for _, c := range out["cases"].([]any) {
			m[c.(map[string]any)["id"].(string)] = true
		}
		return m
	}
	if got := ids(alice); !got[aid] || got[bid] || !got[did] || len(got) != 2 {
		t.Errorf("alice sees %v", got)
	}
	if got := ids(nil); got[aid] || got[bid] || !got[did] {
		t.Errorf("no visitor ID sees %v; want the demo only", got)
	}
	if got := ids(admin); !got[aid] || !got[bid] || !got[did] {
		t.Errorf("admin sees %v", got)
	}
	for _, req := range []struct{ method, path, body string }{
		{"GET", "", ""}, {"PATCH", "", `{"display_name":"mine now"}`}, {"DELETE", "", ""},
		{"POST", "/plan", `{"target":"Thailand"}`}, {"POST", "/export", ""}, {"PUT", "/economics", `{"annual_import_value_usd":1}`},
	} {
		if code, _ := hs.doAs(t, req.method, "/v1/cases/"+bid+req.path, req.body, alice); code != http.StatusNotFound {
			t.Errorf("alice %s bob's case%s = %d, want 404", req.method, req.path, code)
		}
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+did, "", alice); code != http.StatusOK {
		t.Errorf("alice can't open the demo: %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/cases/"+aid, "", alice); code != http.StatusNoContent {
		t.Errorf("alice can't delete her own case: %d", code)
	}
	// Off by default: everyone sees everything, as before.
	open := newHarness(t, "substantial_transformation")
	_, x := open.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), bob)
	if code, _ := open.doAs(t, "GET", "/v1/cases/"+x["id"].(string), "", alice); code != http.StatusOK {
		t.Errorf("ownership off: %d", code)
	}
}

func TestRulingLookupsAreLimitedOnlyOnFetch(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Limits = Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 1000, LookupPerMin: 2}
	})
	ip := map[string]string{"X-Client-IP": "198.51.100.4"}
	for i, id := range []string{"N350001", "N350002"} {
		if code, out := hs.doAs(t, "GET", "/v1/rulings/"+id, "", ip); code != http.StatusOK {
			t.Fatalf("lookup %d = %d %v", i, code, out)
		}
	}
	if code, _ := hs.doAs(t, "GET", "/v1/rulings/N350003", "", ip); code != http.StatusTooManyRequests {
		t.Errorf("third new ruling = %d, want 429", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/rulings/N350001", "", ip); code != http.StatusOK {
		t.Errorf("a ruling already fetched must come from memory, not the limit: %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), ip); code != http.StatusCreated {
		t.Errorf("lookups used up the model budget: %d", code)
	}
}

func TestCaseListIsCapped(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.CasesListLimit = 3 })
	var first string
	for i := 0; i < 5; i++ {
		_, c := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), admin)
		if i == 0 {
			first = c["id"].(string)
		}
	}
	hs.doAs(t, "PUT", "/v1/cases/"+first+"/pin", `{"pinned":true}`, admin)
	_, out := hs.do(t, "GET", "/v1/cases", "")
	rows := out["cases"].([]any)
	if len(rows) != 4 || rows[0].(map[string]any)["id"] != first {
		t.Errorf("got %d rows (want the pinned case plus 3), first %v", len(rows), rows[0].(map[string]any)["id"])
	}
}
