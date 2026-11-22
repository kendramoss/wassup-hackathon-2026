package httpapi

import (
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// alertsNow is the clock revocation alerts read "today" from (tests pin it).
var alertsNow = func() time.Time { return time.Now().UTC() }

// caseRulingAlerts are the read-time revocation alerts on a saved case, each
// labelled with whether the change bears on the case's answer (D531).
func caseRulingAlerts(c store.Case, today time.Time) []domain.RulingAlert {
	d, _ := time.Parse(time.DateOnly, c.Date) // an invalid date reads as zero: no change is "before the case"
	return domain.WithRelevance(domain.RulingAlerts(d, today, c.RulingRefs()), c.Prediction.Origin)
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

// relevanceWord is the watch job's short form of an alert's relevance.
func relevanceWord(a domain.RulingAlert) string {
	switch a.Relevance {
	case domain.RelevanceTouchesAnswer:
		return " (bears on the answer)"
	case domain.RelevanceNotAnswer:
		return " (does not touch the answer directly)"
	case domain.RelevanceHoldingUnchanged:
		return " (origin held unchanged)"
	}
	return " (bearing on the answer not known)"
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
