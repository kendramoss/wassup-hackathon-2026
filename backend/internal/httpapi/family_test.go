package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
)

type fakeFamily struct{ got string }

func (f *fakeFamily) Check(_ context.Context, text string) (familycheck.Result, error) {
	f.got = text
	return familycheck.Result{Family: "led_lights", Name: "LED lights and lamps", Confidence: 0.9, Reason: "An LED lamp."}, nil
}

func TestFamilyCheck(t *testing.T) {
	f := &fakeFamily{}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Family = f })
	code, out := hs.do(t, "POST", "/v1/products/family-check", `{"text":"garden lights","product":{"name":"Solar path light","components":[{"name":"LED PCBA","made_in":"China"}],"plants":[],"operations":[]}}`)
	if code != http.StatusOK || out["result"].(map[string]any)["family"] != "led_lights" || out["prompt_version"] != familycheck.PromptVersion {
		t.Fatalf("check = %d %v", code, out)
	}
	if !strings.Contains(f.got, "garden lights") || !strings.Contains(f.got, "Solar path light") || !strings.Contains(f.got, "LED PCBA") {
		t.Errorf("text sent = %q", f.got)
	}
	if code, _ := hs.do(t, "POST", "/v1/products/family-check", `{"text":"  "}`); code != http.StatusBadRequest {
		t.Errorf("empty = %d", code)
	}
}

func TestFamilyCheckOffIs404(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.do(t, "POST", "/v1/products/family-check", `{"text":"x"}`); code != http.StatusNotFound {
		t.Errorf("off = %d", code)
	}
}
