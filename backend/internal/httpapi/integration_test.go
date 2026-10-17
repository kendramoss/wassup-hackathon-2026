package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// Tests for where two finals features meet (feat/finals-integration).

// A streamed predict (D71) by a signed-in user (D68) files the case in the
// user's workspace and records the user as the actor, as the JSON predict
// does.
func TestStreamedPredictKeepsTeamsOwnership(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.StreamPredict = true })
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create workspace = %d %v", code, ws)
	}
	hs.addToWorkspace(t, ws["id"].(string), "alice", "bob")

	req := httptest.NewRequest("POST", "/v1/cases/predict", strings.NewReader(heatGunBody(t)))
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-User-Token", "tok-alice")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	evs := readEvents(t, rec.Body.String())
	if len(evs) == 0 || evs[len(evs)-1].name != "done" {
		t.Fatalf("stream = %d %s", rec.Code, rec.Body.String())
	}
	id, _ := evs[len(evs)-1].data["id"].(string)

	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("bob")); code != http.StatusOK {
		t.Errorf("workspace member reads the streamed case = %d, want 200", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+id, "", map[string]string{"X-Visitor-ID": "visitor-0123456789abcdef"}); code != http.StatusNotFound {
		t.Errorf("anonymous visitor reads the streamed case = %d, want 404", code)
	}
	_, got := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("alice"))
	hist, _ := got["history"].([]any)
	if len(hist) == 0 || hist[0].(map[string]any)["actor"] != "alice@example.com" {
		t.Errorf("predicted event actor: %v", hist)
	}
}

// A streamed predict sent to a regime engine (USMCA D61, pharma D62)
// reports that engine's stages and closes every stage it opened, also when
// the regime engine fails part-way and the case falls back to out of scope.
func TestStreamedPredictRegimeEngines(t *testing.T) {
	const facts = `{"date":"2026-03-01","facts":"Bolt seals stamped and assembled in Mexico from Chinese steel wire."}`
	failing := realEngine(func(llm.Request) (llm.Response, error) { return llm.Response{}, errors.New("model down") })
	for _, tt := range []struct {
		name, regime, origin string
		set                  func(*Deps)
	}{
		{"usmca answers", "usmca_part102", "China", func(d *Deps) { d.USMCA = realEngine(answerWith("0.9", 0)) }},
		{"usmca fails", "usmca_part102", "China", func(d *Deps) { d.USMCA = failing }},
		{"pharma answers", "pharma_api", "China", func(d *Deps) { d.Pharma = realEngine(answerWith("0.9", 0)) }},
		{"pharma fails", "pharma_api", "China", func(d *Deps) { d.Pharma = failing }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, tt.regime, func(d *Deps) {
				d.StreamPredict = true
				d.Engine = realEngine(answerWith("0.9", 0))
				tt.set(d)
			})
			rec := streamPredict(t, hs, facts)
			evs := readEvents(t, rec.Body.String())
			if len(evs) == 0 || evs[len(evs)-1].name != "done" {
				t.Fatalf("stream = %d %s", rec.Code, rec.Body.String())
			}
			stages := stageList(evs)
			open := map[string]int{}
			for _, s := range stages {
				name, status, _ := strings.Cut(s, " ")
				if status == "started" {
					open[name]++
				} else {
					open[name]--
				}
			}
			for name, n := range open {
				if n != 0 {
					t.Errorf("stage %s started and done do not pair up: %q", name, stages)
				}
			}
			// The main engine and the regime engine each report their stages.
			if n := strings.Count(strings.Join(stages, ","), "engine started"); n != 2 {
				t.Errorf("engine started %d times, want 2 (main and regime engine): %q", n, stages)
			}
			p := evs[len(evs)-1].data["prediction"].(map[string]any)
			if p["origin_country"] != tt.origin {
				t.Errorf("origin = %v, want %s", p["origin_country"], tt.origin)
			}
		})
	}
}

// A BOM version (D67) applied by a workspace member (D68) is visible to the
// workspace and its history names the member who applied it.
func TestBOMVersionByWorkspaceMember(t *testing.T) {
	hs, _, _ := teamsHarness(t, func(d *Deps) { d.BOMVersions = true })
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("create workspace = %d %v", code, ws)
	}
	hs.addToWorkspace(t, ws["id"].(string), "alice", "bob")
	id := hs.newCase(t, user("alice"))

	if code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", map[string]string{"X-Visitor-ID": "visitor-0123456789abcdef"}); code != http.StatusNotFound {
		t.Errorf("anonymous visitor applies a BOM to a workspace case = %d %v, want 404", code, out)
	}
	code, out := hs.uploadBOM(t, id, motorInThailand(t), "true", user("bob"))
	if code != http.StatusOK || out["applied"] != true {
		t.Fatalf("bob applies the BOM version = %d %v", code, out)
	}
	_, got := hs.doAs(t, "GET", "/v1/cases/"+id, "", user("alice"))
	hist, _ := got["history"].([]any)
	var actor any
	for _, e := range hist {
		if ev := e.(map[string]any); ev["kind"] == "bom_updated" {
			actor = ev["actor"]
		}
	}
	if actor != "bob@example.com" {
		t.Errorf("bom_updated actor = %v, want bob@example.com; history %v", actor, hist)
	}
}
