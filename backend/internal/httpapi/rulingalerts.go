package httpapi

import (
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// alertsNow is the clock revocation alerts read "today" from (tests pin it).
var alertsNow = func() time.Time { return time.Now().UTC() }

// caseRulingAlerts are the read-time revocation alerts on a saved case, each
// labelled with whether the change bears on what the case relies on the
// ruling for: the answer, a shown plan's target or the current setup's CBP
// holding (D531, D561).
func caseRulingAlerts(c store.Case, today time.Time) []domain.RulingAlert {
	d, _ := time.Parse(time.DateOnly, c.Date) // an invalid date reads as zero: no change is "before the case"
	return domain.WithRelevanceBy(domain.RulingAlerts(d, today, c.RulingRefs()), c.RelevanceBasis())
}

// rulingAlertsField is the case's ruling_alerts field, or nil when no ruling
// it read or relies on has changed: every other case reads byte for byte as
// before. The stored answer is never changed; rechecked says the ruling
// watch has re-run the case since the latest final change it relies on.
func rulingAlertsField(c store.Case, today time.Time) map[string]any {
	as := caseRulingAlerts(c, today)
	if len(as) == 0 {
		return nil
	}
	out := map[string]any{"counts": domain.CountAlerts(as), "items": as}
	latest := ""
	for _, a := range as {
		if a.Relied() && a.Status == domain.AlertFinal && a.InEffect && a.Date > latest {
			latest = a.Date
		}
	}
	if latest != "" && c.Watch != nil && c.Watch.CheckedAt.Format(time.DateOnly) >= latest {
		out["rechecked_at"] = c.Watch.CheckedAt
	}
	return out
}

// alertCounts is the cases-list badge: nil when no ruling the case relies on
// has a final or proposed change.
func alertCounts(c store.Case, today time.Time) *domain.AlertCounts {
	n := domain.CountAlerts(caseRulingAlerts(c, today))
	if n.Final == 0 && n.Proposed == 0 {
		return nil
	}
	n.ReadOnly = 0
	return &n
}

// relevancePhrase is the short form of an alert's relevance, naming what the
// ruling was judged for (D561): "bears on the answer", "does not touch the
// answer directly", and so on.
func relevancePhrase(a domain.RulingAlert) string {
	what := "the answer"
	switch a.RelevanceRole {
	case domain.RolePlanSupport:
		what = "a shown plan"
	case domain.RoleCurrentSetup:
		what = "the current setup"
	}
	switch a.Relevance {
	case domain.RelevanceTouchesAnswer:
		return "bears on " + what
	case domain.RelevanceNotAnswer:
		return "does not touch " + what + " directly"
	case domain.RelevanceHoldingUnchanged:
		return "origin held unchanged"
	}
	return "bearing on " + what + " not known"
}

// relevanceWord is the watch job's short form of an alert's relevance,
// naming what the ruling was judged for (D561).
func relevanceWord(a domain.RulingAlert) string { return " (" + relevancePhrase(a) + ")" }

// inEffectChanges keeps the final changes already in force: what a letter or
// a shared packet may say CBP has changed. A proposed change, or one dated
// after today, is left out.
func inEffectChanges(as []domain.RulingAlert) []domain.RulingAlert {
	var out []domain.RulingAlert
	for _, a := range as {
		if a.Relied() && a.Status == domain.AlertFinal && a.InEffect {
			out = append(out, a)
		}
	}
	return out
}

// draftRulingChanges are the final, in-effect CBP changes to the rulings a
// draft relies on (#714): the supporting and counter rulings, the rulings the
// prediction cited and the current setup's ruling, each judged against the
// origin the draft asks for. They go in the draft's Notes only, never in the
// letter. Empty ids are skipped.
func draftRulingChanges(ex export.Input, current *planner.KnownOrigin, planTarget string, caseDate, today time.Time) []export.RulingChange {
	var refs []domain.RulingRef
	basis := domain.RelevanceBasis{Predicted: ex.Prediction.Origin}
	if ex.Supporting != "" {
		refs = append(refs, domain.RulingRef{ID: ex.Supporting, Role: domain.RolePlanSupport})
		if planTarget != "" {
			basis.PlanTargets = map[string][]string{ex.Supporting: {planTarget}}
		}
	}
	if ex.Counter != "" {
		refs = append(refs, domain.RulingRef{ID: ex.Counter, Role: domain.RoleCited})
	}
	for _, id := range ex.Prediction.CitedRulings {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleCited})
	}
	if current != nil && current.Ruling != "" {
		refs = append(refs, domain.RulingRef{ID: current.Ruling, Role: domain.RoleCurrentSetup})
		basis.CurrentSetup = current.Country
	}
	var out []export.RulingChange
	for _, a := range inEffectChanges(domain.WithRelevanceBy(domain.RulingAlerts(caseDate, today, refs), basis)) {
		out = append(out, export.RulingChange{Ruling: a.Ruling, Change: a.Note, Relevance: relevancePhrase(a)})
	}
	return out
}

// sharedRulingChanges are the CBP changes to the rulings a shared draft
// relies on that took effect on or after the day it was shared (#714). It
// reads only the FROZEN review draft, never the live case, so a later edit
// of the case cannot move it, and it writes nothing: derived on every read.
//
// The frozen draft holds the rulings the prediction cited and the plan's
// supporting and counter rulings. The current setup's ruling is only text
// inside the frozen letter, not a structured field, so it is not covered.
// Dates are date-only: a change on the sharing day counts (UTC calendar
// dates), whatever the time of day.
func sharedRulingChanges(rv store.BrokerReview, today time.Time) []domain.RulingAlert {
	d := rv.Draft
	basis := domain.RelevanceBasis{Predicted: d.Prediction.Origin}
	var refs []domain.RulingRef
	if p := d.Plan; p != nil {
		if p.Supporting != "" {
			refs = append(refs, domain.RulingRef{ID: p.Supporting, Role: domain.RolePlanSupport})
			basis.PlanTargets = map[string][]string{p.Supporting: {p.PredictedOrigin}}
		}
		if p.Counter != "" {
			refs = append(refs, domain.RulingRef{ID: p.Counter, Role: domain.RoleCited})
		}
	}
	for _, id := range d.Prediction.CitedRulings {
		refs = append(refs, domain.RulingRef{ID: id, Role: domain.RoleCited})
	}
	shared := rv.CreatedAt.UTC().Format(time.DateOnly)
	var out []domain.RulingAlert
	for _, a := range inEffectChanges(domain.WithRelevanceBy(domain.RulingAlerts(time.Time{}, today, refs), basis)) {
		if a.Date >= shared {
			out = append(out, a)
		}
	}
	return out
}

// watchAlert is one case's line in the watch job's ruling_alerts.
type watchAlert struct {
	domain.AlertCounts
	// Rulings are the relied-on rulings with a final change, each as
	// "N… revoked by H… on/effective …".
	Rulings []string `json:"rulings,omitempty"`
}

// watchAlerts lists, by case id, the analysed cases whose relied-on rulings
// CBP has changed or proposed to change (D480). Health probes are skipped.
func watchAlerts(cs []store.Case, today time.Time) map[string]watchAlert {
	out := map[string]watchAlert{}
	for _, c := range cs {
		if c.Prediction.Origin == "" || IsHealthProbeID(c.ID) {
			continue
		}
		as := caseRulingAlerts(c, today)
		n := domain.CountAlerts(as)
		if n.Final == 0 && n.Proposed == 0 {
			continue
		}
		n.ReadOnly = 0
		wa := watchAlert{AlertCounts: n}
		for _, a := range as {
			if a.Relied() && a.Status == domain.AlertFinal {
				wa.Rulings = append(wa.Rulings, a.Ruling+" "+a.Note+relevanceWord(a))
			}
		}
		out[c.ID] = wa
	}
	return out
}
