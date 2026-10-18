package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestExportCFR177Format(t *testing.T) {
	off := newHarness(t, "substantial_transformation")
	_, c := off.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	// Off (the default): the field is refused and the default stays Markdown.
	if code, out := off.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"cfr177","preview":true}`); code != http.StatusBadRequest {
		t.Errorf("cfr177 with the flag off = %d %v, want 400", code, out)
	}
	if code, out := off.do(t, "POST", "/v1/cases/"+id+"/export", `{"preview":true}`); code != http.StatusOK || out["format"] != "markdown" {
		t.Errorf("default export = %d %v", code, out["format"])
	}

	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ExportCFR177 = true })
	_, c = hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id = c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	for _, tc := range []struct {
		body, format string
		code         int
		want         []string
	}{
		{body: `{"plan_index":0,"preview":true}`, format: "markdown", code: 200, want: []string{"[REQUESTER NAME]", "## Requester's position"}},
		{body: `{"plan_index":0,"format":"markdown","preview":true}`, format: "markdown", code: 200, want: []string{"[REQUESTER NAME]"}},
		{body: `{"plan_index":0,"format":"CFR177","preview":true}`, format: "cfr177", code: 200, want: []string{
			"## 2. Port of entry", "proposed change to that setup: make Motor-fan assembly and PCBA (SMT) in Thailand", "origin China (N328954)",
			"- electric motor: Thailand (Rayong plant).", // the move is applied to the component list
			"- power cord: China (China suppliers).", "Manufacturer, Rayong plant (Thailand): [FILL IN", "N347421 — a comparable change",
			"variant facts", "substantial transformation (19 C.F.R. § 134.1(b))",
		}},
		{body: `{"move_key":"pcba@th","format":"cfr177","preview":true}`, format: "cfr177", code: 200, want: []string{"rated this change 0.88, under its 0.92 bar", "- electric motor: China (China suppliers)."}},
		{body: `{"format":"pdf"}`, code: 400},
	} {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", tc.body)
		if code != tc.code {
			t.Errorf("export %s = %d %v, want %d", tc.body, code, out, tc.code)
			continue
		}
		if code != 200 {
			continue
		}
		md, _ := out["content"].(string)
		// The draft's notes are beside the letter, never in it (18 Oct).
		if strings.Contains(md, "Draft for review") || !strings.Contains(joinNotes(out), "Draft for review") {
			t.Errorf("export %s: notes in the letter or missing from notes: %v", tc.body, out["notes"])
		}
		md += "\n" + joinNotes(out)
		if out["format"] != tc.format {
			t.Errorf("export %s format = %v, want %s", tc.body, out["format"], tc.format)
		}
		for _, w := range tc.want {
			if !strings.Contains(md, w) {
				t.Errorf("export %s missing %q:\n%s", tc.body, w, md)
			}
		}
	}

	// Preview records nothing; a real export records the format in history.
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"cfr177","preview":true}`)
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if len(after["history"].([]any)) != len(before["history"].([]any)) || after["updated_at"] != before["updated_at"] {
		t.Error("a cfr177 preview wrote to the case")
	}
	if code, _ := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"cfr177"}`); code != 200 {
		t.Fatalf("cfr177 export = %d", code)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	h := got["history"].([]any)
	if ev := h[len(h)-1].(map[string]any); ev["kind"] != "exported" || !strings.HasPrefix(ev["summary"].(string), "19 CFR 177.2 ruling-request letter for current setup") {
		t.Errorf("last event = %v", ev)
	}
}

func TestExportCFR177OnPinnedCaseDoesNotWriteForVisitors(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.ExportCFR177 = true })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.doAs(t, "PUT", "/v1/cases/"+id+"/pin", `{"pinned":true}`, admin)
	_, before := hs.do(t, "GET", "/v1/cases/"+id, "")
	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"format":"cfr177"}`)
	if code != http.StatusOK || out["format"] != "cfr177" || !strings.Contains(out["content"].(string), "## 1. Interested parties") {
		t.Fatalf("export = %d %v", code, out["format"])
	}
	_, after := hs.do(t, "GET", "/v1/cases/"+id, "")
	if len(after["history"].([]any)) != len(before["history"].([]any)) || after["updated_at"] != before["updated_at"] {
		t.Error("a visitor's cfr177 export wrote to a pinned case")
	}
}

func TestExportCFR177TextileCase(t *testing.T) {
	hs := newHarnessWith(t, "textile_102_21", func(d *Deps) { d.ExportCFR177 = true; d.Textile = &fakeEngine{conf: 0.97} })
	_, c := hs.do(t, "POST", "/v1/cases/predict", `{"date":"2026-02-01","facts":"A knit sweater is knit in China and sewn in Vietnam."}`)
	code, out := hs.do(t, "POST", "/v1/cases/"+c["id"].(string)+"/export", `{"format":"cfr177","preview":true}`)
	md, _ := out["content"].(string)
	if code != 200 || !strings.Contains(md, "determined under 19 C.F.R. § 102.21") || !strings.Contains(md, "Manufacturer(s): [FILL IN") {
		t.Errorf("textile letter %d:\n%s", code, md)
	}
}

// joinNotes is an export response's notes as one string.
func joinNotes(out map[string]any) string {
	var parts []string
	for _, n := range out["notes"].([]any) {
		parts = append(parts, n.(string))
	}
	return strings.Join(parts, "\n")
}
