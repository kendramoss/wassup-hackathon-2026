package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// What old_origin was predicted from (D67): the case's own facts, or the old
// product rendered with the template like the new one.
const (
	factsCase     = "case"
	factsRendered = "rendered"
)

// bomVersion compares a newer bill of materials with the case's product
// (D67): what changed that can change the origin, and, when anything did,
// the prediction for the new product beside the case's own. Nothing is saved
// unless apply is true; then the case takes the new product and prediction
// and records a bom_updated event. Accepts multipart/form-data with a "file"
// (CSV or .xlsx, read as for /v1/products/import) and an "apply" field, or
// JSON {csv, apply}.
func (a *api) bomVersion(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sc, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	src, apply, err := bomUpload(w, r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	defer src.Close()
	if apply && a.refuseDemoEdit(w, r, sc) {
		return
	}
	if sc.Product == nil {
		a.fail(w, r, fmt.Errorf("%w: case %s has no structured product to compare a BOM with", domain.ErrInvalid, sc.ID))
		return
	}
	if sc.Prediction.Origin == "" {
		a.fail(w, r, fmt.Errorf("%w: case %s has not been analysed", domain.ErrInvalid, sc.ID))
		return
	}
	tm, ok := a.d.Templates[sc.Family]
	if !ok {
		a.fail(w, r, fmt.Errorf("%w: a BOM version needs a case family with a template; known: %s", domain.ErrInvalid, a.families()))
		return
	}
	rows, err := bom.ReadRows(src)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, warnings, err := bom.Parse(rows, bom.Options{Name: sc.Product.Name, Description: sc.Product.Description, Family: sc.Family, Template: &tm})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p.HTS = sc.Product.HTS // not in the sheet; keeps the sourced duty rates (D52)
	p = bom.KeepPlantIDs(*sc.Product, p)
	if warnings == nil {
		warnings = []string{}
	}
	changes := bom.Diff(*sc.Product, p)
	out := map[string]any{
		"case_id": sc.ID, "changes": changes, "warnings": warnings, "product": p,
		"old_origin": sc.Prediction.Origin, "old_confidence": sc.Prediction.Confidence,
		"new_origin": sc.Prediction.Origin, "new_confidence": sc.Prediction.Confidence,
		"origin_changed": false, "triage": sc.Triage, "repredicted": false, "applied": false,
		"facts_basis": factsCase, "case_origin": sc.Prediction.Origin,
	}
	if len(changes) == 0 { // nothing that can change the origin: no model call, nothing to save
		a.respond(w, http.StatusOK, out)
		return
	}
	date, err := time.Parse(time.DateOnly, sc.Date)
	if err != nil {
		a.fail(w, r, fmt.Errorf("case %s has no valid date: %w", sc.ID, err))
		return
	}
	facts := planner.Render(p, tm) // ends with one newline, as predict feeds the engine (D20)
	c := domain.Case{ID: sc.ID, Date: date, Facts: facts, Product: &p, Exclude: sc.Exclude}
	if err := c.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	// The new BOM is predicted from rendered facts. When the case's own facts
	// are something else (CBP's wording for a demo, a drafted description,
	// clarify answers), the case's prediction is not a fair "before": the
	// wording alone can move the origin (D16: the heat gun is Thailand on
	// rendered facts, China on CBP's). The old product is then rendered and
	// predicted too, as the plan search's baseline is, and the alert compares
	// rendered old with rendered new.
	var base *domain.Case
	if oldFacts := planner.Render(*sc.Product, tm); strings.TrimSpace(oldFacts) != strings.TrimSpace(sc.Facts) {
		base = &domain.Case{ID: sc.ID, Date: date, Facts: oldFacts, Product: sc.Product, Exclude: sc.Exclude}
	}
	// The comparison is predicted like a scenario (router, engine, triage);
	// an applied version gets the full analysis, clarify included, as predict.
	next := sc
	var label, baseLabel *router.Label
	var basePred domain.Prediction
	var baseErr error
	var wg sync.WaitGroup
	if base != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			basePred, baseLabel, _, _, baseErr = a.classify(r.Context(), *base, nil)
		}()
	}
	if apply {
		err = a.analyse(r, &next, c, nil)
		label = next.Router
	} else {
		var pred domain.Prediction
		var tr domain.Triage
		pred, label, tr, _, err = a.classify(r.Context(), c, nil)
		next.Prediction, next.Triage = pred, tr
	}
	wg.Wait()
	if err == nil {
		err = baseErr
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	oldOrigin := sc.Prediction.Origin
	if base != nil {
		oldOrigin = basePred.Origin
		out["facts_basis"], out["old_origin"], out["old_confidence"] = factsRendered, basePred.Origin, basePred.Confidence
	}
	out["new_origin"], out["new_confidence"], out["triage"], out["repredicted"] = next.Prediction.Origin, next.Prediction.Confidence, next.Triage, true
	out["origin_changed"] = domain.NormalizeCountry(next.Prediction.Origin) != domain.NormalizeCountry(oldOrigin)
	out["prediction"] = next.Prediction
	if !apply {
		a.respond(w, http.StatusOK, out)
		return
	}
	var u usage
	u.trace(next.Prediction.Trace)
	u.label(label)
	if next.Clarify != nil {
		u.trace(next.Clarify.Trace)
	}
	if base != nil {
		u.trace(basePred.Trace)
		u.label(baseLabel)
	}
	pr := next.Prediction
	summary := fmt.Sprintf("new BOM, %d change(s); origin %s, was %s; confidence %.2f, %s",
		len(changes), pr.Origin, sc.Prediction.Origin, pr.Confidence, next.Triage.Route)
	if base != nil {
		summary += fmt.Sprintf("; the old BOM, rendered, gives %s; the facts are now rendered from the BOM, not the case's own wording", basePred.Origin)
	}
	now := time.Now().UTC()
	saved, err := a.d.Cases.Update(r.Context(), sc.ID, func(latest *store.Case) error {
		latest.Product, latest.Facts, latest.UpdatedAt = &p, facts, now
		latest.Prediction, latest.Router, latest.Triage, latest.Clarify = next.Prediction, next.Router, next.Triage, next.Clarify
		// Answers were appended to the old facts, and the plan, target
		// comparison, watch and draft were worked out from the old product, so
		// the case goes back to the origin step (the history keeps them);
		// scenarios are the user's own setups and stay.
		latest.Answers, latest.OriginalFacts = nil, ""
		latest.Plan, latest.Targets, latest.Watch, latest.ExportedAt = nil, nil, nil, nil
		// CBP's ruling was on the previous BOM. Kept, a plan search would tell
		// the verifier CBP had ruled on this product (D17), scenarios would
		// start from it and the draft would cite it.
		s := summary
		if k := latest.CurrentOrigin; k != nil && k.Country != "" {
			s += fmt.Sprintf("; CBP's ruling %s (%s) was on the previous BOM and no longer applies", k.Ruling, k.Country)
		}
		latest.CurrentOrigin = nil
		record(r.Context(), latest, u.on(store.Event{Kind: "bom_updated", Summary: s,
			Provider: pr.Trace.Provider, Model: pr.Trace.ModelID, Prompt: pr.Trace.PromptVersion}, start))
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out["applied"], out["case"] = true, a.caseFields(saved)
	a.respond(w, http.StatusOK, out)
}

// bomUpload reads the BOM and the apply flag from a multipart upload (file,
// apply) or a JSON body {csv, apply}.
func bomUpload(w http.ResponseWriter, r *http.Request) (io.ReadCloser, bool, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			return nil, false, fmt.Errorf("%w: upload: %v", domain.ErrInvalid, err)
		}
		apply := false
		if v := strings.TrimSpace(r.FormValue("apply")); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, false, fmt.Errorf("%w: apply must be true or false, got %q", domain.ErrInvalid, v)
			}
			apply = b
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			return nil, false, fmt.Errorf("%w: a file field with the CSV or Excel BOM is required", domain.ErrInvalid)
		}
		return f, apply, nil
	}
	var in struct {
		CSV   string `json:"csv"`
		Apply bool   `json:"apply"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxUpload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, false, fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err)
	}
	return io.NopCloser(strings.NewReader(in.CSV)), in.Apply, nil
}
