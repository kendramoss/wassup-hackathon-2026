package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// MaxScenarios caps the alternative setups compared on one case.
const MaxScenarios = 3

type scenarioInput struct {
	Label   string          `json:"label"`
	Facts   string          `json:"facts,omitempty"`
	Product *domain.Product `json:"product,omitempty"`
}

type scenariosRequest struct {
	Scenarios []scenarioInput `json:"scenarios"`
	// Economics prices each scenario against the current setup; when omitted,
	// the case's latest economics (from a plan search) are used.
	Economics *planner.Economics `json:"economics,omitempty"`
}

// scenarios predicts up to MaxScenarios alternative production setups for a
// case, side by side, and replaces the case's previous scenarios.
func (a *api) scenarios(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sc, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in scenariosRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Economics != nil {
		if err := validateEconomics(*in.Economics); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	if sc.Economics != nil && (in.Economics == nil || in.Economics.AnnualImportValueUSD <= 0) {
		if err := validateEconomics(*sc.Economics); err != nil { // reused stored economics
			a.fail(w, r, err)
			return
		}
	}
	if len(in.Scenarios) == 0 || len(in.Scenarios) > MaxScenarios {
		a.fail(w, r, fmt.Errorf("%w: give 1 to %d scenarios", domain.ErrInvalid, MaxScenarios))
		return
	}
	date, err := time.Parse(time.DateOnly, sc.Date)
	if err != nil {
		a.fail(w, r, fmt.Errorf("case %s has no valid date: %w", sc.ID, err))
		return
	}
	fp, basis0 := inputFingerprint(sc), scenarioBasis(sc) // what the comparisons are worked out against (D97)
	out := make([]store.Scenario, len(in.Scenarios))
	cases := make([]domain.Case, len(in.Scenarios))
	for i, s := range in.Scenarios {
		label := strings.TrimSpace(s.Label)
		if label == "" {
			label = fmt.Sprintf("Scenario %d", i+1)
		}
		facts, err := a.scenarioFacts(s, sc.Family)
		if err != nil {
			a.fail(w, r, fmt.Errorf("scenario %q: %w", label, err))
			return
		}
		out[i] = store.Scenario{Label: label, Facts: facts, Product: s.Product}
		cases[i] = domain.Case{ID: sc.ID, Date: date, Facts: facts, Product: s.Product, Exclude: sc.Exclude}
		if err := cases[i].Validate(); err != nil {
			a.fail(w, r, fmt.Errorf("scenario %q: %w", label, err))
			return
		}
	}
	if err := a.auditCase(r, sc, "scenarios"); err != nil {
		a.fail(w, r, err)
		return
	}
	errs := make([]error, len(cases))
	labels := make([]*router.Label, len(cases))
	var wg sync.WaitGroup
	for i := range cases {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pred, label, tr, _, err := a.classify(r.Context(), cases[i], nil)
			out[i].Prediction, out[i].Triage, labels[i], errs[i] = pred, tr, label, err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			a.fail(w, r, fmt.Errorf("scenario %q: %w", out[i].Label, err))
			return
		}
	}
	if in.Economics != nil && in.Economics.AnnualImportValueUSD > 0 {
		sc.Economics = in.Economics
	}
	a.compareScenarios(r.Context(), sc, out, true)
	var parts []string
	var u usage
	for i, s := range out {
		parts = append(parts, fmt.Sprintf("%s: %s %.2f", s.Label, s.Prediction.Origin, s.Prediction.Confidence))
		u.trace(s.Prediction.Trace)
		u.label(labels[i])
	}
	ev := u.on(store.Event{Kind: "scenarios", Summary: strings.Join(parts, "; ")}, start)
	now := time.Now().UTC()
	if a.demoReadOnly(r, sc) {
		sc.Scenarios, sc.UpdatedAt = out, now
		record(r.Context(), &sc, ev)
		a.respond(w, http.StatusOK, a.caseFields(r, sc))
		return
	}
	saved, err := a.d.Cases.Update(r.Context(), sc.ID, func(latest *store.Case) error {
		// A new BOM, answers or origin replaced the baseline while this ran:
		// the comparisons belong to the old one, so the save is discarded.
		if err := stillCurrent(latest, fp); err != nil {
			return err
		}
		if scenarioBasis(*latest) != basis0 {
			return errSuperseded
		}
		if in.Economics != nil && in.Economics.AnnualImportValueUSD > 0 {
			latest.Economics = in.Economics
		}
		latest.Scenarios, latest.UpdatedAt = out, now
		record(r.Context(), latest, ev)
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(r, saved))
}

// scenarioFacts is a scenario's facts text: given directly, or rendered from
// a structured product with the case family's template.
func (a *api) scenarioFacts(s scenarioInput, family string) (string, error) {
	var facts string
	switch {
	case strings.TrimSpace(s.Facts) != "" && s.Product != nil:
		return "", fmt.Errorf("%w: give facts or product, not both", domain.ErrInvalid)
	case s.Product != nil:
		if err := s.Product.Validate(); err != nil {
			return "", err
		}
		tm, ok := a.d.Templates[family]
		if !ok {
			return "", fmt.Errorf("%w: a product scenario needs a case family with a template; known: %s", domain.ErrInvalid, a.families())
		}
		facts = planner.Render(*s.Product, tm)
		if !strings.HasSuffix(facts, "\n") { // same format as predict (D20)
			facts += "\n"
		}
	default:
		// The canonical form the case agent's what_if predicts and returns,
		// so Add-as-scenario of its facts stores exactly what was predicted.
		var n int
		var ok bool
		if facts, n, ok = domain.CanonicalFacts(s.Facts); !ok {
			if n == 0 {
				return "", fmt.Errorf("%w: facts or product is required", domain.ErrInvalid)
			}
			return "", fmt.Errorf("%w: facts are %d characters; max %d", domain.ErrInvalid, n, domain.MaxFactsRunes)
		}
	}
	return facts, nil
}

// watch re-runs one case with today's precedents.
func (a *api) watch(w http.ResponseWriter, r *http.Request) {
	sc, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if sc.Prediction.Origin == "" {
		a.fail(w, r, fmt.Errorf("%w: case %s has not been analysed", domain.ErrInvalid, sc.ID))
		return
	}
	if err := a.auditCase(r, sc, "watch"); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.recheck(r.Context(), &sc, !a.demoReadOnly(r, sc)); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(r, sc))
}

// WatchOriginalRegimeUnknownReason is the watch triage reason when the case's
// stored prediction names no known engine: the regime it was made under is
// not known, so a watch result is never auto-accepted against it.
const WatchOriginalRegimeUnknownReason = "regime unavailable: the stored prediction names no known engine, so the watch result cannot be compared with it; an expert reviews"

// watchBasis says why a watch result can or cannot be read as a precedent
// change. Regime and method differences, a failed specialist and a missing
// label are named so they never pass for a ruling-driven origin change.
func watchBasis(sc store.Case, d Dispatch, orig string) string {
	switch {
	case d.Scope == ScopeLabelMissing || orig == "":
		return store.WatchBasisUnverified
	case d.Specialist == SpecialistFailed:
		return store.WatchBasisSpecialistUnavailable
	case d.Regime() != orig:
		return store.WatchBasisRegime
	case d.Result.Prediction.Trace.PromptVersion != sc.Prediction.Trace.PromptVersion:
		return store.WatchBasisMethod
	}
	return store.WatchBasisPrecedent
}

// watchAll re-checks every analysed case (for a scheduler) and lists the
// cases whose origin changed.
func (a *api) watchAll(w http.ResponseWriter, r *http.Request) {
	if err := a.auditBegin(r, auditStep{Tenant: "platform"}); err != nil {
		a.fail(w, r, err)
		return
	}
	cs, err := a.d.Cases.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	checked, changed, failed := 0, []string{}, []string{}
	basis := map[string]string{} // why each changed case changed (store.WatchBasis*)
	for _, c := range cs {
		if c.Prediction.Origin == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c store.Case) {
			defer wg.Done()
			defer func() { <-sem }()
			err := a.recheck(r.Context(), &c, true)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				a.d.Logger.WarnContext(r.Context(), "watch failed", caseAttr(c.ID), "err", logsafe.Err(err))
				failed = append(failed, c.ID)
				return
			}
			checked++
			if c.Watch.Status == store.WatchChanged {
				changed = append(changed, c.ID)
				basis[c.ID] = c.Watch.Basis
			}
		}(c)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"checked": checked, "changed": changed, "changed_basis": basis, "failed": failed})
}

// recheck predicts the case again with every precedent dated before today
// (the original run only saw rulings dated before the case date), records
// which of the precedents used are newer than the case, and stores the result.
func (a *api) recheck(ctx context.Context, sc *store.Case, save bool) error {
	caseDate, err := time.Parse(time.DateOnly, sc.Date)
	if err != nil {
		return fmt.Errorf("case %s has no valid date: %w", sc.ID, err)
	}
	start := time.Now()
	now := start.UTC()
	fp := inputFingerprint(*sc) // what this check is worked out from (D97)
	c := domain.Case{ID: sc.ID, Date: now, Facts: sc.Facts, Product: sc.Product, Exclude: sc.Exclude}
	// The stored label is pinned: the watch answers the question on the
	// regime the case was analysed under, never a fresh routing (router
	// drift). One legacy control stays unpinned: no stored label, no router
	// configured and a prediction made by the general engine, which is the
	// no-router behaviour as before. A specialist or unknown prediction with
	// no label is pinned to nothing (label missing: review), never routed or
	// read as general.
	orig := PredictionRegime(sc.Prediction)
	var pin *pinned
	if sc.Router != nil || a.d.Router != nil || orig != RegimeGeneral {
		pin = &pinned{label: sc.Router}
	}
	d, err := a.dispatch(ctx, c, nil, pin)
	if err != nil {
		return err
	}
	res := d.Result
	tr, _ := a.decide(d)
	if orig == "" && tr.Route == domain.RouteAuto {
		// The stored prediction names no known engine, so this result cannot
		// be compared with it; the verdict does not auto-accept.
		tr = domain.Triage{Route: domain.RouteNeedsReview, Reason: WatchOriginalRegimeUnknownReason}
	}
	newer := []string{}
	if a.d.Precedents != nil {
		for _, id := range res.Prediction.Precedents {
			if p, ok := a.d.Precedents.Get(id); ok && p.Date.After(caseDate) {
				newer = append(newer, id)
			}
		}
	}
	status := store.WatchUnchanged
	if res.Prediction.Origin != sc.Prediction.Origin {
		status = store.WatchChanged
	}
	// A background check is not case activity: UpdatedAt (list order) stays.
	watch := &store.Watch{CheckedAt: now, Status: status, Prediction: res.Prediction, Newer: newer,
		Basis: watchBasis(*sc, d, orig), Regime: d.Regime(), OriginalRegime: orig, Scope: string(d.Scope), Specialist: string(d.Specialist), Triage: &tr}
	var u usage
	u.trace(res.Prediction.Trace)
	ev := u.on(store.Event{At: now, Kind: "watched", Summary: fmt.Sprintf("%s: origin %s with today's precedents (%d newer rulings used); %s, %s", status, res.Prediction.Origin, len(newer), watch.Basis, tr.Route),
		Provider: res.Prediction.Trace.Provider, Model: res.Prediction.Trace.ModelID, Prompt: res.Prediction.Trace.PromptVersion}, start)
	if !save {
		sc.Watch = watch
		record(ctx, sc, ev)
		return nil
	}
	saved, err := a.d.Cases.Update(ctx, sc.ID, func(latest *store.Case) error {
		// A new BOM or answers replaced the inputs while this ran: the
		// result belongs to the old facts, so it is discarded, not attached.
		if err := stillCurrent(latest, fp); err != nil {
			return err
		}
		latest.Watch = watch
		record(ctx, latest, ev)
		return nil
	})
	if err != nil {
		return err
	}
	*sc = saved
	return nil
}
