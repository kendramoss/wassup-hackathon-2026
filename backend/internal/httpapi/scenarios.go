package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
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
	sc, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in scenariosRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
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
	errs := make([]error, len(cases))
	var wg sync.WaitGroup
	for i := range cases {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pred, _, tr, _, err := a.classify(r.Context(), cases[i])
			out[i].Prediction, out[i].Triage, errs[i] = pred, tr, err
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
	if e := sc.Economics; e != nil {
		current := sc.Prediction.Origin
		if sc.CurrentOrigin != nil && sc.CurrentOrigin.Country != "" {
			current = sc.CurrentOrigin.Country
		}
		for i := range out {
			cmp := planner.Compare(*e, sc.Product, out[i].Product, current, out[i].Prediction.Origin)
			out[i].Comparison = &cmp
		}
	}
	sc.Scenarios, sc.UpdatedAt = out, time.Now().UTC()
	if err := a.d.Cases.Put(r.Context(), sc); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(sc))
}

// scenarioFacts is a scenario's facts text: given directly, or rendered from
// a structured product with the case family's template.
func (a *api) scenarioFacts(s scenarioInput, family string) (string, error) {
	facts := strings.TrimSpace(s.Facts)
	switch {
	case facts != "" && s.Product != nil:
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
	case facts == "":
		return "", fmt.Errorf("%w: facts or product is required", domain.ErrInvalid)
	}
	if !strings.HasSuffix(facts, "\n") { // same format as predict (D20)
		facts += "\n"
	}
	return facts, nil
}

// watch re-runs one case with today's precedents.
func (a *api) watch(w http.ResponseWriter, r *http.Request) {
	sc, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if sc.Prediction.Origin == "" {
		a.fail(w, r, fmt.Errorf("%w: case %s has not been analysed", domain.ErrInvalid, sc.ID))
		return
	}
	if err := a.recheck(r.Context(), &sc); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(sc))
}

// watchAll re-checks every analysed case (for a scheduler) and lists the
// cases whose origin changed.
func (a *api) watchAll(w http.ResponseWriter, r *http.Request) {
	cs, err := a.d.Cases.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	checked, changed, failed := 0, []string{}, []string{}
	for _, c := range cs {
		if c.Prediction.Origin == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c store.Case) {
			defer wg.Done()
			defer func() { <-sem }()
			err := a.recheck(r.Context(), &c)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				a.d.Logger.WarnContext(r.Context(), "watch failed", "case", c.ID, "err", err)
				failed = append(failed, c.ID)
				return
			}
			checked++
			if c.Watch.Status == store.WatchChanged {
				changed = append(changed, c.ID)
			}
		}(c)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"checked": checked, "changed": changed, "failed": failed})
}

// recheck predicts the case again with every precedent dated before today
// (the original run only saw rulings dated before the case date), records
// which of the precedents used are newer than the case, and stores the result.
func (a *api) recheck(ctx context.Context, sc *store.Case) error {
	caseDate, err := time.Parse(time.DateOnly, sc.Date)
	if err != nil {
		return fmt.Errorf("case %s has no valid date: %w", sc.ID, err)
	}
	now := time.Now().UTC()
	c := domain.Case{ID: sc.ID, Date: now, Facts: sc.Facts, Product: sc.Product, Exclude: sc.Exclude}
	res, err := a.d.Engine.Predict(ctx, c)
	if err != nil {
		return err
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
	sc.Watch = &store.Watch{CheckedAt: now, Status: status, Prediction: res.Prediction, Newer: newer}
	return a.d.Cases.Put(ctx, *sc)
}
