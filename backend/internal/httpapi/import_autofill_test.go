package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
)

type scriptedFamily struct {
	res   familycheck.Result
	calls int
}

func (s *scriptedFamily) Check(context.Context, string) (familycheck.Result, error) {
	s.calls++
	return s.res, nil
}

const autofillCSV = "Part,Country,Plant,Group,Plant capabilities\n" +
	"electric motor,China,China suppliers,motor_fan_assembly,pcba\n" +
	"housing,Vietnam,Binh Duong,housing_moulding,motor_fan_assembly\n"

func importJSON(t *testing.T, hs harness, body map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	code, out := hs.do(t, "POST", "/v1/products/import", string(b))
	if code != http.StatusOK {
		t.Fatalf("import = %d %v", code, out)
	}
	return out
}

func TestImportNameAndFamilyFromSheet(t *testing.T) {
	f := &scriptedFamily{}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Family = f })
	csv := "Product name,Family,Part,Country,Group\nHair dryer 1800 W,Motorised tools & small appliances,electric motor,China,motor_fan_assembly\n,,housing,Vietnam,housing_moulding\n"
	out := importJSON(t, hs, map[string]any{"csv": csv})
	if out["family"] != "motorised_tools_small_appliances" || out["family_source"] != "file" || out["name_source"] != "file" ||
		out["product"].(map[string]any)["name"] != "Hair dryer 1800 W" || f.calls != 0 {
		t.Errorf("sheet import = %v (checks %d)", out, f.calls)
	}
}

func TestImportFamilyAndNameFromAI(t *testing.T) {
	f := &scriptedFamily{res: familycheck.Result{Family: "motorised_tools_small_appliances", Confidence: 0.9, Reason: "Motor and housing.", ProductName: "Desk fan"}}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Family = f })
	out := importJSON(t, hs, map[string]any{"csv": autofillCSV})
	p := out["product"].(map[string]any)
	if out["family"] != "motorised_tools_small_appliances" || out["family_source"] != "ai" || out["family_reason"] != "Motor and housing." ||
		out["name_source"] != "ai" || p["name"] != "Desk fan" {
		t.Errorf("ai import = %v", out)
	}
	// Groups map to the chosen template: no "not in the template" warnings.
	for _, w := range out["warnings"].([]any) {
		if s := w.(string); len(s) > 0 && s[:4] == "row " {
			t.Errorf("group warning after AI family: %s", s)
		}
	}
	// A given family and name win; the check isn't asked for the family.
	given := importJSON(t, hs, map[string]any{"csv": autofillCSV, "family": "led_lights", "name": "My lamp"})
	if given["family"] != "led_lights" || given["family_source"] != "given" || given["name_source"] != nil || given["product"].(map[string]any)["name"] != "My lamp" {
		t.Errorf("given = %v", given)
	}
	// A weak match is not applied.
	f.res = familycheck.Result{Family: "led_lights", Confidence: 0.3, Reason: "?"}
	weak := importJSON(t, hs, map[string]any{"csv": autofillCSV, "name": "Thing"})
	if weak["family"] != "" || weak["family_source"] != nil {
		t.Errorf("weak = %v", weak)
	}
}
