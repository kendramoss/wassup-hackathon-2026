package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/bom"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
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
	// The comparison is predicted like a scenario (router, engine, triage);
	// an applied version gets the full analysis, clarify included, as predict.
	next := sc
	var label *router.Label
	if apply {
		if err := a.analyse(r, &next, c); err != nil {
			a.fail(w, r, err)
			return
		}
		label = next.Router
	} else {
		pred, l, tr, _, err := a.classify(r.Context(), c)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		next.Prediction, next.Triage, label = pred, tr, l
	}
	out["new_origin"], out["new_confidence"], out["triage"], out["repredicted"] = next.Prediction.Origin, next.Prediction.Confidence, next.Triage, true
	out["origin_changed"] = domain.NormalizeCountry(next.Prediction.Origin) != domain.NormalizeCountry(sc.Prediction.Origin)
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
	pr := next.Prediction
	ev := u.on(store.Event{Kind: "bom_updated", Summary: fmt.Sprintf("new BOM, %d change(s); origin %s, was %s; confidence %.2f, %s",
		len(changes), pr.Origin, sc.Prediction.Origin, pr.Confidence, next.Triage.Route),
		Provider: pr.Trace.Provider, Model: pr.Trace.ModelID, Prompt: pr.Trace.PromptVersion}, start)
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
		latest.Record(ev)
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
