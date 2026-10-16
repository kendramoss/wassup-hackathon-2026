package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func TestBatchTriage(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	body := `{"items":[
		{"name":"Heat gun","facts":"A heat gun assembled in China with a motor from China.","date":"2022-06-01"},
		{"name":"Fan","facts":"A fan with the motor in Thailand, assembled in Thailand."},
		{"name":"Bad date","facts":"Something","date":"01/02/2022"},
		{"name":"","facts":""}]}`
	code, out := hs.do(t, "POST", "/v1/cases/batch", body)
	if code != http.StatusOK {
		t.Fatalf("batch = %d %v", code, out)
	}
	rows := out["rows"].([]any)
	if len(rows) != 4 {
		t.Fatalf("rows = %v", rows)
	}
	r0, r1 := rows[0].(map[string]any), rows[1].(map[string]any)
	if r0["predicted_origin"] != "China" || r0["triage_route"] != "auto" || r0["error"] != nil || r0["case_id"] == nil || r0["cost_usd"] == nil {
		t.Errorf("row 0 = %v", r0)
	}
	if r1["predicted_origin"] != "Thailand" {
		t.Errorf("row 1 = %v", r1)
	}
	if e, _ := rows[2].(map[string]any)["error"].(string); !strings.Contains(e, "YYYY-MM-DD") {
		t.Errorf("row 2 error = %v", rows[2])
	}
	if e, _ := rows[3].(map[string]any)["error"].(string); e == "" {
		t.Errorf("empty facts not an error: %v", rows[3])
	}
	s := out["summary"].(map[string]any)
	if s["total"] != 4.0 || s["auto"] != 2.0 || s["failed"] != 2.0 || s["out_of_scope"] != 0.0 {
		t.Errorf("summary = %v", s)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	if n := len(list["cases"].([]any)); n != 2 {
		t.Errorf("saved %d cases, want 2", n)
	}
	names := map[string]bool{}
	for _, c := range list["cases"].([]any) {
		names[c.(map[string]any)["display_name"].(string)] = true
	}
	if !names["Heat gun"] || !names["Fan"] {
		t.Errorf("saved cases are not named after their rows: %v", names)
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/batch", `{"items":[]}`); code != http.StatusBadRequest {
		t.Errorf("empty batch = %d", code)
	}
	many := make([]map[string]string, MaxBatch+1)
	for i := range many {
		many[i] = map[string]string{"facts": "x"}
	}
	b, _ := json.Marshal(map[string]any{"items": many})
	if code, _ := hs.do(t, "POST", "/v1/cases/batch", string(b)); code != http.StatusBadRequest {
		t.Errorf("oversize batch = %d", code)
	}
}

func TestBatchOutOfScopeAndCSV(t *testing.T) {
	hs := newHarness(t, "textile_102_21")
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "portfolio.csv")
	_, _ = fw.Write([]byte("SKU,Description,Date\nG-1,\"Knit polyester gaiters, knit in China\",2024-03-01\n,,\n"))
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/cases/batch", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv batch = %d %s", rec.Code, rec.Body)
	}
	rows := out["rows"].([]any)
	r := rows[0].(map[string]any)
	if len(rows) != 1 || r["name"] != "G-1" || r["out_of_scope"] != true || r["triage_route"] != "needs_review" {
		t.Errorf("rows = %v", rows)
	}
	if out["summary"].(map[string]any)["out_of_scope"] != 1.0 {
		t.Errorf("summary = %v", out["summary"])
	}
}

func TestBatchUsesTheBulkLimit(t *testing.T) {
	l := newLimiter(Limits{LightPerMin: 100, HeavyPerMin: 100, GlobalPerMin: 30})
	if ok, _ := l.allow("a", bulk); !ok { // takes 20 of 30
		t.Fatal("first batch refused")
	}
	if ok, _ := l.allow("b", bulk); ok {
		t.Fatal("second batch allowed with 10 left")
	}
}

func TestDeepHealth(t *testing.T) {
	cs := store.NewMemory()
	broken := errors.New("rpc error: code = PermissionDenied desc = Missing or insufficient permissions.")
	for _, tt := range []struct {
		name   string
		checks []HealthCheck
		code   int
	}{
		{"all ok", []HealthCheck{StoreCheck(cs), {Name: "model", Run: func(context.Context) error { return nil }}}, http.StatusOK},
		{"store denied", []HealthCheck{{Name: "case_store", Run: func(context.Context) error { return broken }}}, http.StatusServiceUnavailable},
	} {
		hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Checks, d.Cases = tt.checks, cs })
		code, out := hs.do(t, "GET", "/health?deep=1", "")
		if code != tt.code {
			t.Errorf("%s: %d %v", tt.name, code, out)
		}
		checks, _ := out["checks"].(map[string]any)
		if tt.code != http.StatusOK {
			c := checks["case_store"].(map[string]any)
			if c["ok"] != false || !strings.Contains(c["error"].(string), "PermissionDenied") || out["status"] != "degraded" {
				t.Errorf("%s: %v", tt.name, out)
			}
		} else if checks["case_store"].(map[string]any)["ok"] != true {
			t.Errorf("%s: %v", tt.name, out)
		}
		if code, plain := hs.do(t, "GET", "/health", ""); code != 200 || plain["checks"] != nil {
			t.Errorf("plain health = %d %v", code, plain)
		}
	}
	if _, err := cs.Get(context.Background(), HealthProbeID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("probe case left behind: %v", err)
	}
}
