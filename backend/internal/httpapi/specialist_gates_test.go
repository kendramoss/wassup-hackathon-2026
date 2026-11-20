package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// #300 residual: the specialist bar and the fallback bar are independent
// mandatory-review gates. A case is auto only when every applicable gate
// passes; neither fast path overrides the other.
const gateFallbackModel = "gemini-fallback"

type gateBar struct {
	name string
	v    float64
}

// Confidence is 0.97: 0 is the default-off bar, 0.99 is not met, 0.90 is met.
var gateBars = []gateBar{{"off", 0}, {"below", 0.99}, {"met", 0.90}}

func (b gateBar) passes() bool { return b.v > 0 && 0.97 >= b.v }

func TestSpecialistAndFallbackGatesCompose(t *testing.T) {
	for _, rc := range regimeCases {
		for _, fallback := range []bool{false, true} {
			for _, sb := range gateBars {
				for _, fb := range gateBars {
					model, who := "", "primary"
					if fallback {
						model, who = gateFallbackModel, "fallback"
					}
					name := fmt.Sprintf("%s/%s/spec_%s/fb_%s", rc.name, who, sb.name, fb.name)
					t.Run(name, func(t *testing.T) {
						wantAuto := sb.passes() && (!fallback || fb.passes())
						spec := &specialistFake{version: rc.version, origin: rc.origin, model: model}
						hs := newHarnessWith(t, rc.regime, func(d *Deps) {
							rc.wire(d, spec)
							setSpecialistBar(d, rc.regime, sb.v)
							d.FallbackFastPath = fb.v
						})
						var d Deps
						newHarnessWith(t, rc.regime, func(dd *Deps) {
							rc.wire(dd, spec)
							setSpecialistBar(dd, rc.regime, sb.v)
							dd.FallbackFastPath = fb.v
							d = *dd
						})
						check := func(layer, route, reason string) {
							t.Helper()
							if (route == "auto") != wantAuto {
								t.Errorf("%s: route %s reason %q, want auto=%v", layer, route, reason, wantAuto)
							}
							if !strings.HasPrefix(reason, rc.prefix) && !(sb.passes() && fallback) {
								t.Errorf("%s: reason %q lacks specialist prefix", layer, reason)
							}
							if !sb.passes() && !strings.Contains(reason, "an expert reviews every") && !strings.Contains(reason, "below") {
								t.Errorf("%s: specialist gate reason missing: %q", layer, reason)
							}
							if fallback && !fb.passes() && !strings.Contains(reason, triage.FallbackReason(model)) {
								t.Errorf("%s: fallback gate reason missing: %q", layer, reason)
							}
							if !fallback && strings.Contains(reason, "fallback") {
								t.Errorf("%s: primary answer mentions fallback: %q", layer, reason)
							}
						}
						code, out := hs.do(t, "POST", "/v1/cases/predict", dispatchBody)
						if code != http.StatusCreated {
							t.Fatalf("predict = %d %v", code, out)
						}
						tr := out["triage"].(map[string]any)
						check("http", tr["route"].(string), tr["reason"].(string))
						if out["clarify"] != nil {
							t.Errorf("clarify ran on a specialist case")
						}
						cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "gaiters"})
						if err != nil {
							t.Fatal(err)
						}
						check("classify", string(cl.Triage.Route), cl.Triage.Reason)
						if cl.Specialist != SpecialistOK || cl.Regime() != rc.regime || PredictionRegime(cl.Result.Prediction) != rc.regime || cl.InScope {
							t.Errorf("regime state = %v %q %q inScope %v", cl.Specialist, cl.Regime(), PredictionRegime(cl.Result.Prediction), cl.InScope)
						}
					})
				}
			}
		}
	}
}

func setSpecialistBar(d *Deps, regime string, v float64) {
	switch regime {
	case router.RegimeTextile:
		d.TextileFastPath = v
	case router.RegimeUSMCA:
		d.USMCAFastPath = v
	default:
		d.PharmaFastPath = v
	}
}

// Controls: the general engine keeps its own fallback gate and the primary
// general path is accepted at the ordinary bar.
func TestGeneralFallbackGateControl(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		for _, fb := range gateBars {
			t.Run(fmt.Sprintf("fallback_%v/fb_%s", fallback, fb.name), func(t *testing.T) {
				model := ""
				if fallback {
					model = gateFallbackModel
				}
				var d Deps
				newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
					dd.Engine = &fakeEngine{conf: 0.97, modelID: model}
					dd.FallbackFastPath = fb.v
					d = *dd
				})
				cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "heat gun"})
				if err != nil {
					t.Fatal(err)
				}
				want := !fallback || fb.passes()
				if (cl.Triage.Route == domain.RouteAuto) != want {
					t.Errorf("triage = %+v, want auto=%v", cl.Triage, want)
				}
			})
		}
	}
}
