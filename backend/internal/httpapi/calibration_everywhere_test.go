package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Issue #402 / D161: every surface that reports a prediction's confidence
// must name the calibrated probability when the map attached one, with the
// model's own number labelled. The reproduction in the issue showed batch,
// BOM version, watch, Ask and history giving only the stated 0.85 next to a
// reason that quoted calibrated 0.60.
func TestCalibratedConfidenceInBatchBOMWatchAskAndHistory(t *testing.T) {
	m := calibrate.Must()
	const stated, calibrated, newStated, newCalibrated = 0.85, 0.6, 0.97, 0.9355

	hs := bomHarness(t, func(d *Deps) {
		d.Calibration, d.CalibratedFastPath = &m, m.Bar
	})
	hs.engine.conf = stated
	assertCal := func(where string, p map[string]any, s, c float64) {
		t.Helper()
		if p == nil {
			t.Fatalf("%s: no prediction", where)
		}
		if p["confidence"] != s || p["calibrated_confidence"] != c || p["calibration"] != m.Version {
			t.Errorf("%s: prediction %v, want stated %v, calibrated %v, map %s", where, p, s, c, m.Version)
		}
	}

	// 1. Batch rows carry the value their triage reason quotes.
	code, out := hs.do(t, "POST", "/v1/cases/batch", `{"items":[{"name":"Heat gun","facts":"A heat gun assembled in China with a motor from China.","date":"2022-06-01"}]}`)
	if code != http.StatusOK {
		t.Fatalf("batch = %d %v", code, out)
	}
	row := out["rows"].([]any)[0].(map[string]any)
	if row["calibrated_confidence"] != calibrated || row["calibration"] != m.Version {
		t.Errorf("batch row = %v", row)
	}
	if reason, _ := row["reason"].(string); !strings.Contains(reason, "calibrated confidence 0.60") || !strings.Contains(reason, "model stated 0.85") {
		t.Errorf("batch reason = %q", reason)
	}
	batchID, _ := row["case_id"].(string)
	if batchID == "" {
		t.Fatal("batch saved no case")
	}

	// 2. The saved case's history and the Ask tool's get_case text name both.
	_, c := hs.do(t, "GET", "/v1/cases/"+batchID, "")
	assertCal("stored case", c["prediction"].(map[string]any), stated, calibrated)
	text := ""
	for _, ev := range c["history"].([]any) {
		e := ev.(map[string]any)
		if e["kind"] == "predicted" {
			text, _ = e["summary"].(string)
		}
	}
	if !strings.Contains(text, "calibrated 0.60; model stated 0.85") {
		t.Errorf("predicted history = %q", text)
	}
	cal := calibrated
	askText := agentCaseText(store.Case{ID: batchID, Date: "2026-01-14", Facts: "heat gun",
		Prediction: domain.Prediction{Origin: "China", Confidence: stated, CalibratedConfidence: &cal, Calibration: m.Version}})
	if !strings.Contains(askText, "calibrated 0.60; model stated 0.85") {
		t.Errorf("agentCaseText = %q", askText)
	}

	// 3. BOM version: old and new calibrated values and the map version ride
	// with the stated ones, so the panel need not pair 0.92 with a reason
	// that says 0.90. BOM comparison needs a structured product, which batch
	// rows do not carry, so use a normal predicted case.
	code, out = hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	if code != http.StatusCreated {
		t.Fatalf("predict = %d %v", code, out)
	}
	id := out["id"].(string)
	assertCal("predicted case", out["prediction"].(map[string]any), stated, calibrated)
	// The BOM re-prediction uses a different stated value, so a top-level
	// calibrated field copied from the case can be distinguished from one
	// copied from the new prediction.
	hs.engine.conf = newStated
	code, out = hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code != http.StatusOK {
		t.Fatalf("bom-version = %d %v", code, out)
	}
	if out["new_calibrated_confidence"] != newCalibrated || out["old_calibrated_confidence"] != calibrated || out["calibration"] != m.Version {
		t.Errorf("bom-version = %v", out)
	}
	assertCal("bom-version prediction", out["prediction"].(map[string]any), newStated, newCalibrated)
	if tr, _ := out["triage"].(map[string]any); tr != nil {
		if reason, _ := tr["reason"].(string); !strings.Contains(reason, "calibrated confidence 0.94") || !strings.Contains(reason, "model stated 0.97") {
			t.Errorf("bom triage reason = %q", reason)
		}
	}

	// 4. The watch result's prediction already carried the value; the UI now
	// draws it. Keep the API guard here as well.
	hs.engine.later = "Thailand"
	code, out = hs.do(t, "POST", "/v1/cases/"+id+"/watch", "")
	if code != http.StatusOK {
		t.Fatalf("watch = %d %v", code, out)
	}
	w := out["watch"].(map[string]any)
	if w["status"] != "changed" {
		t.Fatalf("watch status = %v", w["status"])
	}
	assertCal("watch", w["prediction"].(map[string]any), newStated, newCalibrated)
}
