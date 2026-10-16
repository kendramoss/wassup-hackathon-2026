package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// heatGunBOM is backend/testdata/heatgun_bom.csv with each from replaced by
// the to after it (pairs).
func heatGunBOM(t *testing.T, pairs ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "heatgun_bom.csv"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	for i := 0; i+1 < len(pairs); i += 2 {
		if !strings.Contains(s, pairs[i]) {
			t.Fatalf("sample BOM has no %q", pairs[i])
		}
		s = strings.Replace(s, pairs[i], pairs[i+1], 1)
	}
	return s
}

// motorInThailand is the heat-gun BOM after the motor supplier moved to the
// Rayong plant.
func motorInThailand(t *testing.T) string {
	return heatGunBOM(t, "electric motor,China,China suppliers", "electric motor,Thailand,Rayong plant")
}

// motorOrigin makes the fake engine answer Thailand when the electric motor
// is made there (and China otherwise), so a BOM change moves the origin.
func motorOrigin(p domain.Product) string {
	for _, c := range p.Components {
		if c.Name == "electric motor" && c.MadeIn == "Thailand" {
			return "Thailand"
		}
	}
	return ""
}

// uploadBOM posts csv as a multipart file, with apply when not empty.
func (hs harness) uploadBOM(t *testing.T, id, csv, apply string, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "bom-v2.csv")
	_, _ = fw.Write([]byte(csv))
	if apply != "" {
		_ = mw.WriteField("apply", apply)
	}
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/bom-version", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func bomHarness(t *testing.T, adjust func(*Deps)) harness {
	t.Helper()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.BOMVersions = true
		if adjust != nil {
			adjust(d)
		}
	})
	hs.engine.byProduct = motorOrigin
	return hs
}

func TestBOMVersionIsOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code, out := hs.uploadBOM(t, c["id"].(string), motorInThailand(t), "", nil); code != http.StatusNotFound || out["code"] != "not_found" {
		t.Errorf("BOM_VERSIONS off: %d %v, want 404", code, out)
	}
}

func TestBOMVersionMotorCountryChange(t *testing.T) {
	hs := bomHarness(t, nil)
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	calls := hs.engine.calls

	code, out := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code != http.StatusOK {
		t.Fatalf("bom-version %d %v", code, out)
	}
	changes := out["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes = %v", changes)
	}
	ch := changes[0].(map[string]any)
	if ch["kind"] != "country_changed" || ch["name"] != "electric motor" || ch["from_country"] != "China" || ch["to_country"] != "Thailand" ||
		ch["from_plant"] != "China suppliers" || ch["to_plant"] != "Rayong plant" {
		t.Errorf("change = %v", ch)
	}
	if out["old_origin"] != "China" || out["new_origin"] != "Thailand" || out["origin_changed"] != true || out["new_confidence"] != 0.97 ||
		out["repredicted"] != true || out["applied"] != false || out["disclaimer"] != domain.Disclaimer {
		t.Errorf("comparison = %v", out)
	}
	if tr := out["triage"].(map[string]any); tr["route"] != "auto" {
		t.Errorf("triage = %v", tr)
	}
	// One engine call, on the new product rendered with the case's family
	// template, its date and exclusions.
	if hs.engine.calls != calls+1 {
		t.Errorf("engine calls = %d, want %d", hs.engine.calls, calls+1)
	}
	last := hs.engine.last
	if last.Product == nil || motorOrigin(*last.Product) != "Thailand" || last.Date.Format("2006-01-02") != "2026-01-14" || len(last.Exclude) != 2 ||
		!strings.Contains(last.Facts, "electric motor") || !strings.HasSuffix(last.Facts, "\n") || last.Product.HTS != "8419.89.95" {
		t.Errorf("engine got %+v", last)
	}
	// Plant IDs are the case's own (th, cn), not the upload's.
	for _, pl := range out["product"].(map[string]any)["plants"].([]any) {
		if id := pl.(map[string]any)["id"]; id != "th" && id != "cn" {
			t.Errorf("plant id %v not kept from the case", id)
		}
	}
	// Without apply the case is unchanged.
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if after["prediction"].(map[string]any)["origin_country"] != "China" || len(after["history"].([]any)) != len(before["history"].([]any)) ||
		after["updated_at"] != before["updated_at"] || after["plan"] == nil || motorCountry(after) != "China" {
		t.Errorf("a comparison changed the case: %v", after)
	}

	// The same BOM again: nothing that can change the origin, no model call.
	calls = hs.engine.calls
	code, out = hs.uploadBOM(t, id, heatGunBOM(t), "", nil)
	if code != http.StatusOK || len(out["changes"].([]any)) != 0 || out["repredicted"] != false || out["origin_changed"] != false ||
		out["new_origin"] != "China" || hs.engine.calls != calls {
		t.Errorf("unchanged BOM: %d %v (engine calls %d -> %d)", code, out, calls, hs.engine.calls)
	}

	// apply: the case takes the new product and prediction and records it.
	code, out = hs.uploadBOM(t, id, motorInThailand(t), "true", nil)
	if code != http.StatusOK || out["applied"] != true || out["origin_changed"] != true {
		t.Fatalf("apply %d %v", code, out)
	}
	_, after = hs.do(t, "GET", "/v1/cases/"+id, "")
	h := after["history"].([]any)
	ev := h[len(h)-1].(map[string]any)
	if after["prediction"].(map[string]any)["origin_country"] != "Thailand" || motorCountry(after) != "Thailand" || after["plan"] != nil ||
		after["step"] != "origin" || ev["kind"] != "bom_updated" || !strings.Contains(ev["summary"].(string), "1 change(s); origin Thailand, was China") ||
		ev["model_id"] != "gemini-4-argon" || ev["cost_usd"] == nil || len(h) != len(before["history"].([]any))+1 {
		t.Errorf("applied case = prediction %v, step %v, plan %v, event %v", after["prediction"], after["step"], after["plan"], ev)
	}
	if !strings.Contains(after["facts"].(string), "electric motor") || after["product"].(map[string]any)["hts"] != "8419.89.95" {
		t.Errorf("applied facts/product = %v / %v", after["facts"], after["product"])
	}
}

func motorCountry(c map[string]any) string {
	for _, x := range c["product"].(map[string]any)["components"].([]any) {
		if m := x.(map[string]any); m["name"] == "electric motor" {
			return m["made_in"].(string)
		}
	}
	return ""
}

func TestBOMVersionRespectsDemosOwnershipAndLimits(t *testing.T) {
	hs := bomHarness(t, func(d *Deps) {
		d.CaseOwnership = true
		d.Limits = Limits{LightPerMin: 2, HeavyPerMin: 1, GlobalPerMin: 100}
	})
	alice := map[string]string{"X-Visitor-ID": "alice-visitor-id-0001", "X-Client-IP": "203.0.113.1"}
	bob := map[string]string{"X-Visitor-ID": "bob-visitor-id-000002", "X-Client-IP": "203.0.113.2"}
	_, demo := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), admin)
	did := demo["id"].(string)
	hs.doAs(t, "PUT", "/v1/cases/"+did+"/pin", `{"pinned":true}`, admin)
	_, mine := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), admin)
	// Pinned demo (D57): a visitor may compare but not apply.
	if code, _ := hs.uploadBOM(t, did, motorInThailand(t), "true", bob); code != http.StatusForbidden {
		t.Errorf("visitor apply on a demo = %d, want 403", code)
	}
	if code, out := hs.uploadBOM(t, did, motorInThailand(t), "", bob); code != http.StatusOK || out["new_origin"] != "Thailand" {
		t.Errorf("visitor compare on a demo = %d %v", code, out)
	}
	if _, d := hs.doAs(t, "GET", "/v1/cases/"+did, "", admin); d["prediction"].(map[string]any)["origin_country"] != "China" {
		t.Errorf("the demo changed: %v", d["prediction"])
	}
	if code, out := hs.uploadBOM(t, did, motorInThailand(t), "true", admin); code != http.StatusOK || out["applied"] != true {
		t.Errorf("admin apply on a demo = %d %v", code, out)
	}
	// Ownership (D58): another visitor's case is a 404.
	if code, _ := hs.uploadBOM(t, mine["id"].(string), motorInThailand(t), "", alice); code != http.StatusNotFound {
		t.Errorf("alice on the admin's unpinned case = %d, want 404", code)
	}
	// Rate-limited light: bob has used 2 of 2.
	if code, _ := hs.uploadBOM(t, did, motorInThailand(t), "", bob); code != http.StatusTooManyRequests {
		t.Errorf("third bom-version = %d, want 429", code)
	}
}

func TestBOMVersionErrors(t *testing.T) {
	hs := bomHarness(t, nil)
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	_, factsOnly := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-02-01","facts":"A heat gun assembled in Thailand."}`)
	tests := []struct {
		name, id, csv, apply string
		want                 int
	}{
		{"apply not a boolean", id, motorInThailand(t), "sure", 400},
		{"no country column", id, "Part,Factory\nmotor,A\n", "", 400},
		{"case without a product", factsOnly["id"].(string), motorInThailand(t), "", 400},
		{"unknown case", "nope", motorInThailand(t), "", 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, out := hs.uploadBOM(t, tt.id, tt.csv, tt.apply, nil); code != tt.want {
				t.Errorf("status %d %v, want %d", code, out, tt.want)
			}
		})
	}
	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/bom-version", strings.NewReader("--x--"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("upload without a file = %d", rec.Code)
	}
	// JSON {csv, apply} works as for /v1/products/import.
	body, _ := json.Marshal(map[string]any{"csv": motorInThailand(t)})
	if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/bom-version", string(body)); code != 200 || out["new_origin"] != "Thailand" {
		t.Errorf("json body: %d %v", code, out)
	}
}
