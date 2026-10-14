package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// Predictor predicts origin (engine.Engine).
type Predictor interface {
	Predict(ctx context.Context, c domain.Case) (engine.Result, error)
}

// Router labels family and regime (router.Router).
type Router interface {
	Classify(ctx context.Context, facts string) (router.Label, error)
}

// Planner searches plans (planner.Planner).
type Planner interface {
	Plan(ctx context.Context, req planner.Request) (planner.Result, error)
}

// RulingSource returns a ruling's text (cross.Client).
type RulingSource interface {
	Ruling(ctx context.Context, id string) (cross.Ruling, error)
}

// Deps are the API's collaborators. Router and Planner may be nil (the
// endpoints then degrade: no scope label / 503 for plans).
type Deps struct {
	Engine        Predictor
	Router        Router
	Planner       Planner
	Rulings       RulingSource
	Cases         store.Cases
	Templates     map[string]planner.Template
	Summary       func() (evalsummary.Summary, error)
	Demos         []demos.Demo
	FastPathConf  float64
	Provider      string
	EngineModel   string
	RouterModel   string
	VerifierModel string
	Logger        *slog.Logger
}

// maxBody caps request bodies (a product with dozens of parts is ~10 KB).
const maxBody = 256 << 10

type api struct {
	d          Deps
	rulingMu   sync.Mutex
	rulingMemo map[string]rulingDTO
}

func (a *api) meta() map[string]any {
	return map[string]any{
		"model_ids":       map[string]string{"engine": a.d.EngineModel, "router": a.d.RouterModel, "verifier": a.d.VerifierModel},
		"prompt_versions": map[string]string{"engine": engine.PromptVersion, "router": router.PromptVersion, "verifier": verifier.PromptVersion},
		"llm_provider":    a.d.Provider,
		"disclaimer":      domain.Disclaimer,
	}
}

func (a *api) respond(w http.ResponseWriter, status int, fields map[string]any) {
	out := a.meta()
	for k, v := range fields {
		out[k] = v
	}
	writeJSON(w, status, out)
}

// fail maps errors to the error format; internal details are logged, not
// returned.
func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *llm.APIError
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, planner.ErrUnknownFamily):
		writeError(w, http.StatusBadRequest, "invalid_input", err.Error(), nil)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, cross.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, llm.ErrBadResponse):
		a.d.Logger.ErrorContext(r.Context(), "model bad response", "request_id", RequestID(r.Context()), "err", err)
		writeError(w, http.StatusBadGateway, "model_bad_response", "the model returned an unusable answer; try again", nil)
	case errors.Is(err, llm.ErrUnavailable), errors.As(err, &apiErr), errors.Is(err, context.DeadlineExceeded):
		a.d.Logger.ErrorContext(r.Context(), "model unavailable", "request_id", RequestID(r.Context()), "err", err)
		writeError(w, http.StatusBadGateway, "model_unavailable", "the model provider is unavailable; try again", nil)
	default:
		a.d.Logger.ErrorContext(r.Context(), "internal error", "request_id", RequestID(r.Context()), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error", nil)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: request body: %v", domain.ErrInvalid, err)
	}
	return nil
}

type predictRequest struct {
	Date          planner.Date         `json:"date"`
	Facts         string               `json:"facts"`
	Product       *domain.Product      `json:"product"`
	Family        string               `json:"family"`
	Exclude       []string             `json:"exclude_rulings"`
	CurrentOrigin *planner.KnownOrigin `json:"current_origin"`
}

func (a *api) predict(w http.ResponseWriter, r *http.Request) {
	var in predictRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	facts := strings.TrimSpace(in.Facts)
	if in.Product != nil {
		if err := in.Product.Validate(); err != nil {
			a.fail(w, r, err)
			return
		}
		if facts == "" {
			tm, ok := a.d.Templates[in.Family]
			if !ok {
				a.fail(w, r, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, in.Family, a.families()))
				return
			}
			facts = planner.Render(*in.Product, tm)
		}
	}
	for _, id := range in.Exclude {
		if !domain.ValidRulingID(id) {
			a.fail(w, r, fmt.Errorf("%w: exclude_rulings: %q is not a CBP ruling number", domain.ErrInvalid, id))
			return
		}
	}
	c := domain.Case{ID: newCaseID(), Date: in.Date.Time, Facts: facts, Product: in.Product, Exclude: in.Exclude}
	if err := c.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}

	// Router and engine run concurrently; the router only labels scope and
	// never changes the engine's answer.
	var label *router.Label
	var wg sync.WaitGroup
	if a.d.Router != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := a.d.Router.Classify(r.Context(), facts)
			if err != nil {
				a.d.Logger.WarnContext(r.Context(), "router failed; scope unknown", "request_id", RequestID(r.Context()), "err", err)
				return
			}
			label = &l
		}()
	}
	res, err := a.d.Engine.Predict(r.Context(), c)
	wg.Wait()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	inScope, scopeReason := true, ""
	if label != nil {
		inScope, scopeReason = label.Scope()
	}
	sc := store.Case{
		ID: c.ID, Date: in.Date.Format(time.DateOnly), Facts: facts, Family: in.Family, Product: in.Product,
		Exclude: in.Exclude, CurrentOrigin: in.CurrentOrigin, Prediction: res.Prediction,
		Triage: triage.Decide(res.Prediction, a.d.FastPathConf, inScope, scopeReason), Router: label, CreatedAt: time.Now().UTC(),
	}
	if err := a.d.Cases.Put(r.Context(), sc); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusCreated, caseFields(sc))
}

func caseFields(c store.Case) map[string]any {
	return map[string]any{
		"id": c.ID, "date": c.Date, "facts": c.Facts, "product": c.Product, "family": c.Family,
		"exclude_rulings": c.Exclude, "current_origin": c.CurrentOrigin, "prediction": c.Prediction,
		"triage": c.Triage, "router": c.Router, "plan": c.Plan, "created_at": c.CreatedAt,
	}
}

func (a *api) getCase(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, caseFields(c))
}

type planRequest struct {
	Target       string                `json:"target"`
	Economics    planner.Economics     `json:"economics"`
	ActualChange *planner.ActualChange `json:"actual_change"`
}

func (a *api) plan(w http.ResponseWriter, r *http.Request) {
	if a.d.Planner == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "planner is not configured", nil)
		return
	}
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in planRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Product == nil || c.Family == "" {
		a.fail(w, r, fmt.Errorf("%w: plan search needs a case created from a structured product with a family", domain.ErrInvalid))
		return
	}
	date, _ := time.Parse(time.DateOnly, c.Date)
	res, err := a.d.Planner.Plan(r.Context(), planner.Request{
		CaseID: c.ID, Date: planner.Date{Time: date}, Family: c.Family, Target: in.Target,
		Product: *c.Product, Economics: in.Economics, Exclude: c.Exclude, CurrentOrigin: c.CurrentOrigin,
		ActualChange: in.ActualChange,
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	c.Plan = &res
	if err := a.d.Cases.Put(r.Context(), c); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "plan": res})
}

type exportRequest struct {
	PlanIndex *int `json:"plan_index"`
}

func (a *api) export(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in exportRequest
	if r.ContentLength != 0 {
		if err := decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	name := ""
	if c.Product != nil {
		name = c.Product.Name
	}
	ex := export.Input{ProductName: name, Facts: c.Facts, Requested: c.Prediction.Origin, Prediction: c.Prediction}
	if c.CurrentOrigin != nil {
		ex.CurrentRuled = strings.TrimSpace(c.CurrentOrigin.Country + " (" + c.CurrentOrigin.Ruling + ")")
	}
	if in.PlanIndex != nil {
		if c.Plan == nil || *in.PlanIndex < 0 || *in.PlanIndex >= len(c.Plan.Plans) {
			a.fail(w, r, fmt.Errorf("%w: plan_index %d: run a plan search first and pick a shown plan", domain.ErrInvalid, *in.PlanIndex))
			return
		}
		p := c.Plan.Plans[*in.PlanIndex]
		ex.Facts, ex.PlanSummary, ex.Requested, ex.Prediction = p.VariantFacts, p.Description, p.Prediction.Origin, p.Prediction
		if p.Verdict != nil {
			ex.Supporting, ex.Counter = p.Verdict.SupportingRuling, p.Verdict.CounterRuling
		}
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "format": "markdown", "content": export.Markdown(ex)})
}

type rulingDTO struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	URL     string `json:"url"`
}

func (a *api) ruling(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.ValidRulingID(id) {
		a.fail(w, r, fmt.Errorf("%w: %q is not a CBP ruling number", domain.ErrInvalid, id))
		return
	}
	a.rulingMu.Lock()
	dto, ok := a.rulingMemo[id]
	a.rulingMu.Unlock()
	if !ok {
		cr, err := a.d.Rulings.Ruling(r.Context(), id)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		date := ""
		if d, err := cr.Date(); err == nil {
			date = d.Format(time.DateOnly)
		}
		dto = rulingDTO{ID: cr.RulingNumber, Date: date, Subject: cr.Subject, Text: cr.Text, URL: domain.RulingURL(id)}
		a.rulingMu.Lock()
		if len(a.rulingMemo) < 500 {
			a.rulingMemo[id] = dto
		}
		a.rulingMu.Unlock()
	}
	writeJSON(w, http.StatusOK, dto)
}

func (a *api) evalSummary(w http.ResponseWriter, r *http.Request) {
	s, err := a.d.Summary()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *api) listDemos(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"demos": demos.Summaries(a.d.Demos)})
}

func (a *api) getDemo(w http.ResponseWriter, r *http.Request) {
	for _, d := range a.d.Demos {
		if d.ID == r.PathValue("id") {
			writeJSON(w, http.StatusOK, d)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "no such demo", nil)
}

func (a *api) families() string {
	var out []string
	for f := range a.d.Templates {
		out = append(out, f)
	}
	return strings.Join(out, ", ")
}

func newCaseID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "case_" + hex.EncodeToString(b)
}
