package httpapi

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// withCalibration fills in the calibrated probability, at read time, on a
// case saved before the calibration map existed (D180). Only predictions the
// map was fitted for get one (engine-v1 answered by the map's model, no
// vote); a stored value is never replaced. It is arithmetic on the stored
// stated confidence, with no model call, and the case in the store is not
// changed: the copy returned shares nothing that is modified with c.
func (a *api) withCalibration(c store.Case) store.Case {
	if a.d.Calibration == nil {
		return c
	}
	fill, result := a.fillCalibration, a.calibratedResult
	c.Prediction = fill(c.Prediction)
	if c.Plan != nil {
		r := result(*c.Plan)
		c.Plan = &r
	}
	if c.Targets != nil {
		t := *c.Targets
		if t.Results != nil {
			t.Results = make(map[string]planner.Result, len(c.Targets.Results))
			for k, r := range c.Targets.Results {
				t.Results[k] = result(r)
			}
		}
		t.Ranked = append([]store.TargetSummary(nil), c.Targets.Ranked...)
		for i := range t.Ranked {
			if b := t.Ranked[i].Best; b != nil {
				cp := *b
				cp.Prediction = fill(cp.Prediction)
				t.Ranked[i].Best = &cp
			}
		}
		c.Targets = &t
	}
	return c
}

// calibratedResult is withCalibration for one plan search (a copy). The plan
// and compare handlers apply it when a search finishes, so the response, the
// streamed events and the stored result carry the same calibrated value a
// later read shows (D251).
func (a *api) calibratedResult(r planner.Result) planner.Result {
	if a.d.Calibration == nil {
		return r
	}
	plans := func(ps []domain.Plan) []domain.Plan {
		if ps == nil {
			return nil
		}
		out := append([]domain.Plan(nil), ps...)
		for i := range out {
			out[i].Prediction = a.fillCalibration(out[i].Prediction)
		}
		return out
	}
	r.Baseline.Prediction = a.fillCalibration(r.Baseline.Prediction)
	r.Plans, r.Rejected = plans(r.Plans), plans(r.Rejected)
	return r
}

// calibratedPlan is withCalibration for one streamed plan (a copy).
func (a *api) calibratedPlan(p *domain.Plan) *domain.Plan {
	if a.d.Calibration == nil || p == nil {
		return p
	}
	cp := *p
	cp.Prediction = a.fillCalibration(cp.Prediction)
	return &cp
}

// fillCalibration is withCalibration for one prediction.
func (a *api) fillCalibration(p domain.Prediction) domain.Prediction {
	m := a.d.Calibration
	if m == nil || p.CalibratedConfidence != nil || p.Origin == "" || !m.Applies(p) {
		return p
	}
	return m.Apply(p)
}

// calibratedScenarios is withCalibration for scenario rows (a copy).
func (a *api) calibratedScenarios(ss []store.Scenario) []store.Scenario {
	if a.d.Calibration == nil || ss == nil {
		return ss
	}
	out := append([]store.Scenario(nil), ss...)
	for i := range out {
		out[i].Prediction = a.fillCalibration(out[i].Prediction)
	}
	return out
}
