package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
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
	PlanWithProgress(ctx context.Context, req planner.Request, progress func(planner.Event)) (planner.Result, error)
}

// Clarifier reviews low-confidence predictions (clarify.Clarifier).
type Clarifier interface {
	Review(ctx context.Context, facts string, p domain.Prediction, precedents []domain.Ruling) (clarify.Result, error)
}

// PrecedentStore looks up rulings shown to the engine.
type PrecedentStore interface {
	Get(id string) (domain.Ruling, bool)
}

// RulingSource returns a ruling's text (cross.Client).
type RulingSource interface {
	Ruling(ctx context.Context, id string) (cross.Ruling, error)
}

// Deps are the API's collaborators. Router and Planner may be nil (the
// endpoints then degrade: no scope label / 503 for plans).
type Deps struct {
	Engine       Predictor
	Router       Router
	Planner      Planner
	Clarifier    Clarifier
	Agent        CaseAgent
	Precedents   PrecedentStore
	Rulings      RulingSource
	Cases        store.Cases
	Templates    map[string]planner.Template
	Summary      func() (evalsummary.Summary, error)
	Demos        []demos.Demo
	FastPathConf float64
	// PlanConfidence is the verifier bar for shown plans (named in below-bar drafts).
	PlanConfidence float64
	Provider       string
	EngineModel    string
	RouterModel    string
	VerifierModel  string
	Logger         *slog.Logger
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
		"prompt_versions": map[string]string{"engine": engine.PromptVersion, "router": router.PromptVersion, "verifier": verifier.PromptVersion, "clarify": clarify.PromptVersion, "answers": clarify.AnswersVersion, "case_agent": caseagent.PromptVersion},
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
	status, code, msg := a.errorFor(r, err)
	writeError(w, status, code, msg, nil)
}

// errorFor maps an error to status, code and a client-safe message, logging
// server-side causes.
func (a *api) errorFor(r *http.Request, err error) (int, string, string) {
	var apiErr *llm.APIError
	switch {
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, planner.ErrUnknownFamily):
		return http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, store.ErrNotFound), errors.Is(err, cross.ErrNotFound):
		return http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, llm.ErrBadResponse):
		a.d.Logger.ErrorContext(r.Context(), "model bad response", "request_id", RequestID(r.Context()), "err", err)
		return http.StatusBadGateway, "model_bad_response", "the model returned an unusable answer; try again"
	case errors.Is(err, llm.ErrUnavailable), errors.As(err, &apiErr), errors.Is(err, context.DeadlineExceeded):
		a.d.Logger.ErrorContext(r.Context(), "model unavailable", "request_id", RequestID(r.Context()), "err", err)
		return http.StatusBadGateway, "model_unavailable", "the model provider is unavailable; try again"
	default:
		a.d.Logger.ErrorContext(r.Context(), "internal error", "request_id", RequestID(r.Context()), "err", err)
		return http.StatusInternalServerError, "internal", "internal error"
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
	// Feed facts in the format the engine was benchmarked on: corpus facts
	// files (and rendered products) end with one newline (D20).
	if facts != "" && !strings.HasSuffix(facts, "\n") {
		facts += "\n"
	}
	c := domain.Case{ID: newCaseID(), Date: in.Date.Time, Facts: facts, Product: in.Product, Exclude: in.Exclude}
	if err := c.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}

	now := time.Now().UTC()
	sc := store.Case{ID: c.ID, Date: in.Date.Format(time.DateOnly), Facts: facts, Family: in.Family, Product: in.Product,
		Exclude: in.Exclude, CurrentOrigin: in.CurrentOrigin, CreatedAt: now, UpdatedAt: now}
	if err := a.analyse(r, &sc, c); err != nil {
		a.fail(w, r, err)
		return
	}
	sc.Record(predictionEvent("predicted", sc))
	if err := a.d.Cases.Put(r.Context(), sc); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusCreated, a.caseFields(sc))
}

// classify runs router and engine concurrently and decides triage. The router
// labels scope only; it never changes the engine's answer.
func (a *api) classify(ctx context.Context, c domain.Case) (domain.Prediction, *router.Label, domain.Triage, bool, error) {
	var label *router.Label
	var wg sync.WaitGroup
	if a.d.Router != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := a.d.Router.Classify(ctx, c.Facts)
			if err != nil {
				a.d.Logger.WarnContext(ctx, "router failed; scope unknown", "request_id", RequestID(ctx), "err", err)
				return
			}
			label = &l
		}()
	}
	res, err := a.d.Engine.Predict(ctx, c)
	wg.Wait()
	if err != nil {
		return domain.Prediction{}, nil, domain.Triage{}, false, err
	}
	inScope, scopeReason := true, ""
	if label != nil {
		inScope, scopeReason = label.Scope()
	}
	return res.Prediction, label, triage.Decide(res.Prediction, a.d.FastPathConf, inScope, scopeReason), inScope, nil
}

// analyse runs router and engine (concurrently), triage and, below the fast
// path, the clarify review; it fills sc. The router and clarify never change
// the engine's answer.
func (a *api) analyse(r *http.Request, sc *store.Case, c domain.Case) error {
	pred, label, tr, inScope, err := a.classify(r.Context(), c)
	if err != nil {
		return err
	}
	sc.Prediction, sc.Router, sc.Triage = pred, label, tr
	sc.Clarify = nil
	if sc.Triage.Route == domain.RouteNeedsReview && inScope && a.d.Clarifier != nil && a.d.Precedents != nil {
		var ps []domain.Ruling
		for _, id := range pred.Precedents {
			if p, ok := a.d.Precedents.Get(id); ok {
				ps = append(ps, p)
			}
		}
		cr, err := a.d.Clarifier.Review(r.Context(), c.Facts, pred, ps)
		if err != nil {
			a.d.Logger.WarnContext(r.Context(), "clarify failed; staying needs_review", "request_id", RequestID(r.Context()), "err", err)
		} else {
			sc.Clarify = &cr
			sc.Triage = triage.WithClarify(sc.Triage, !cr.OnPoint, len(cr.Questions))
		}
	}
	return nil
}

type answersRequest struct {
	Answers []store.Answer `json:"answers"`
}

// toClarify converts stored answers for clarify.AppendAnswers.
func toClarify(as []store.Answer) []clarify.Answer {
	out := make([]clarify.Answer, len(as))
	for i, x := range as {
		out[i] = clarify.Answer{Question: x.Question, Answer: x.Answer}
	}
	return out
}

// answers appends the requester's answers to the facts and re-runs the
// analysis on the same case.
func (a *api) answers(w http.ResponseWriter, r *http.Request) {
	sc, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in answersRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(in.Answers) == 0 {
		a.fail(w, r, fmt.Errorf("%w: answers are required", domain.ErrInvalid))
		return
	}
	for _, x := range in.Answers {
		if strings.TrimSpace(x.Question) == "" || strings.TrimSpace(x.Answer) == "" {
			a.fail(w, r, fmt.Errorf("%w: each answer needs question and answer", domain.ErrInvalid))
			return
		}
	}
	if _, ok := clarify.AppendAnswers("", toClarify(in.Answers)); !ok {
		a.fail(w, r, fmt.Errorf("%w: no answer adds a fact (all unknown); nothing to re-analyse", domain.ErrInvalid))
		return
	}
	if sc.OriginalFacts == "" {
		sc.OriginalFacts = sc.Facts
	}
	sc.Answers = append(sc.Answers, in.Answers...)
	facts, _ := clarify.AppendAnswers(sc.OriginalFacts, toClarify(sc.Answers))
	date, _ := time.Parse(time.DateOnly, sc.Date)
	c := domain.Case{ID: sc.ID, Date: date, Facts: facts, Product: sc.Product, Exclude: sc.Exclude}
	if err := c.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	sc.Facts, sc.UpdatedAt = facts, time.Now().UTC()
	if err := a.analyse(r, &sc, c); err != nil {
		a.fail(w, r, err)
		return
	}
	ev := predictionEvent("answered", sc)
	ev.Summary = fmt.Sprintf("%d answer(s) added; ", len(in.Answers)) + ev.Summary
	sc.Record(ev)
	if err := a.d.Cases.Put(r.Context(), sc); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(sc))
}

// precedentRef is one earlier ruling the engine was shown, with CBP's held
// origin (null when the ruling has no single held origin).
type precedentRef struct {
	ID      string  `json:"id"`
	Date    string  `json:"date"`
	Subject string  `json:"subject"`
	Origin  *string `json:"origin"`
	URL     string  `json:"url"`
}

func (a *api) precedentRefs(ids []string) []precedentRef {
	out := make([]precedentRef, 0, len(ids))
	for _, id := range ids {
		ref := precedentRef{ID: id, URL: domain.RulingURL(id)}
		if a.d.Precedents != nil {
			if p, ok := a.d.Precedents.Get(id); ok {
				ref.Date, ref.Subject, ref.Origin = p.Date.Format(time.DateOnly), p.Subject, nonEmpty(p.Origin)
			}
		}
		out = append(out, ref)
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (a *api) caseFields(c store.Case) map[string]any {
	return map[string]any{
		"id": c.ID, "date": c.Date, "facts": c.Facts, "product": c.Product, "family": c.Family,
		"exclude_rulings": c.Exclude, "current_origin": c.CurrentOrigin, "prediction": c.Prediction,
		"triage": c.Triage, "router": c.Router, "plan": c.Plan, "clarify": c.Clarify, "answers": c.Answers, "created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt, "step": c.Step(), "precedents_detail": a.precedentRefs(c.Prediction.Precedents),
		"scenarios": a.scenarioFields(c.Scenarios), "watch": a.watchFields(c.Watch), "economics": c.Economics, "chat": chatField(c.Chat), "history": historyField(c.History),
		"targets": targetsField(c.Targets),
	}
}

func historyField(h []store.Event) []store.Event {
	if h == nil {
		return []store.Event{}
	}
	return h
}

// predictionEvent records an analysis with its model and prompt.
func predictionEvent(kind string, c store.Case) store.Event {
	p := c.Prediction
	return store.Event{Kind: kind, Summary: fmt.Sprintf("origin %s, confidence %.2f, %s", p.Origin, p.Confidence, c.Triage.Route),
		Provider: p.Trace.Provider, Model: p.Trace.ModelID, Prompt: p.Trace.PromptVersion}
}

func planEvent(res planner.Result) store.Event {
	sum := fmt.Sprintf("target %s: %d moves searched, %d shown", res.Target, res.Stats.MovesSearched, len(res.Plans))
	if len(res.Plans) > 0 {
		sum += "; best: " + res.Plans[0].Description
	}
	if res.Stats.VerifierCached > 0 {
		sum += fmt.Sprintf("; %d/%d precedent checks from cache", res.Stats.VerifierCached, res.Stats.VerifierRuns)
	}
	return store.Event{Kind: "plan_searched", Summary: sum, Provider: res.Baseline.Prediction.Trace.Provider,
		Model: res.Baseline.Prediction.Trace.ModelID, Prompt: res.Baseline.Prediction.Trace.PromptVersion}
}

func chatField(t []store.ChatTurn) []store.ChatTurn {
	if t == nil {
		return []store.ChatTurn{}
	}
	return t
}

// targetsField is the ranked comparison without the full per-target results,
// which can be large; drafts read them from the store.
func targetsField(t *store.TargetComparison) any {
	if t == nil {
		return nil
	}
	return map[string]any{"checked_at": t.CheckedAt, "ranked": t.Ranked}
}

func (a *api) scenarioFields(ss []store.Scenario) []map[string]any {
	out := make([]map[string]any, len(ss))
	for i, s := range ss {
		out[i] = map[string]any{"label": s.Label, "facts": s.Facts, "product": s.Product, "prediction": s.Prediction,
			"triage": s.Triage, "precedents_detail": a.precedentRefs(s.Prediction.Precedents), "comparison": s.Comparison}
	}
	return out
}

func (a *api) watchFields(w *store.Watch) any {
	if w == nil {
		return nil
	}
	return map[string]any{"checked_at": w.CheckedAt, "status": w.Status, "prediction": w.Prediction,
		"newer_rulings": a.precedentRefs(w.Newer), "precedents_detail": a.precedentRefs(w.Prediction.Precedents)}
}

// caseSummary is one row of the cases list.
type caseSummary struct {
	ID          string   `json:"id"`
	ProductName string   `json:"product_name"`
	Date        string   `json:"date"`
	Origin      *string  `json:"predicted_origin"`
	Route       *string  `json:"triage_route"`
	Confidence  *float64 `json:"confidence"`
	Step        string   `json:"step"`
	UpdatedAt   string   `json:"updated_at"`
	Watch       *string  `json:"watch_status"` // null until re-checked; unchanged | changed
	Scenarios   int      `json:"scenario_count"`
}

func (a *api) listCases(w http.ResponseWriter, r *http.Request) {
	cs, err := a.d.Cases.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	rows := make([]caseSummary, 0, len(cs))
	for _, c := range cs {
		row := caseSummary{ID: c.ID, Date: c.Date, Step: c.Step(), UpdatedAt: c.UpdatedAt.Format(time.RFC3339), Scenarios: len(c.Scenarios)}
		if c.Watch != nil {
			row.Watch = &c.Watch.Status
		}
		if c.Product != nil {
			row.ProductName = c.Product.Name
		} else {
			row.ProductName = firstWords(c.Facts, 8)
		}
		if c.Prediction.Origin != "" {
			o, route, conf := c.Prediction.Origin, string(c.Triage.Route), c.Prediction.Confidence
			row.Origin, row.Route, row.Confidence = &o, &route, &conf
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": rows})
}

// firstWords names a facts-only case by its opening words.
func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		return strings.Join(f[:n], " ") + "…"
	}
	return strings.Join(f, " ")
}

func (a *api) getCase(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(c))
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
	req := planner.Request{
		CaseID: c.ID, Date: planner.Date{Time: date}, Family: c.Family, Target: in.Target,
		Product: *c.Product, Economics: in.Economics, Exclude: c.Exclude, CurrentOrigin: c.CurrentOrigin,
		ActualChange: in.ActualChange,
	}
	if err := req.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		a.planStream(w, r, c, req)
		return
	}
	res, err := a.d.Planner.Plan(r.Context(), req)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	c.Plan, c.UpdatedAt = &res, time.Now().UTC()
	c.Economics = economicsFrom(in.Economics, c.Economics)
	c.Record(planEvent(res))
	if err := a.d.Cases.Put(r.Context(), c); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "plan": res})
}

// economicsFrom keeps the latest economics the user gave (an empty request
// value leaves the stored one).
func economicsFrom(e planner.Economics, old *planner.Economics) *planner.Economics {
	if e.AnnualImportValueUSD > 0 {
		return &e
	}
	return old
}

// keepAlive is how often an idle plan stream sends an SSE comment, so proxies
// and Cloud Run keep the connection open.
var keepAlive = 10 * time.Second

// planStream runs the plan search as server-sent events: start {target,
// moves}, baseline {plan}, move_started {key}, move_done {key, plan}, then
// done (the same body as the JSON response) or error {code, message,
// retryable}. Comments (": keep-alive") are sent every keepAlive.
func (a *api) planStream(w http.ResponseWriter, r *http.Request, c store.Case, req planner.Request) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // the search outlives the server's default write timeout
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	var mu sync.Mutex
	write := func(chunk string) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = io.WriteString(w, chunk)
		_ = rc.Flush()
	}
	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		write("event: " + event + "\ndata: " + string(b) + "\n\n")
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(keepAlive)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				write(": keep-alive\n\n")
			case <-stop:
				return
			}
		}
	}()
	res, err := a.d.Planner.PlanWithProgress(r.Context(), req, func(e planner.Event) {
		switch e.Kind {
		case planner.EventStarted:
			send("start", map[string]any{"target": e.Target, "moves": e.Moves, "total": e.Total})
		case planner.EventBaseline:
			send("baseline", map[string]any{"plan": e.Plan})
		case planner.EventMoveStarted:
			send("move_started", map[string]any{"key": e.Move, "done": e.Done, "total": e.Total})
		case planner.EventMoveDone:
			send("move_done", map[string]any{"key": e.Move, "plan": e.Plan, "done": e.Done, "total": e.Total})
		}
	})
	if err == nil {
		c.Plan, c.UpdatedAt = &res, time.Now().UTC()
		c.Economics = economicsFrom(req.Economics, c.Economics)
		c.Record(planEvent(res))
		err = a.d.Cases.Put(r.Context(), c)
	}
	if err != nil {
		status, code, msg := a.errorFor(r, err)
		send("error", map[string]any{"code": code, "message": msg, "retryable": status >= 500})
		return
	}
	out := a.meta()
	out["case_id"], out["plan"] = c.ID, res
	send("done", out)
}

type exportRequest struct {
	PlanIndex *int `json:"plan_index"`
	// MoveKey picks a plan by move, among shown plans or rejected plans whose
	// precedent check was below the bar.
	MoveKey string `json:"move_key"`
	// Target, with MoveKey, picks from that country's search in the latest
	// country comparison instead of the latest plan search.
	Target string `json:"target"`
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
	if in.PlanIndex != nil && in.MoveKey != "" {
		a.fail(w, r, fmt.Errorf("%w: give plan_index or move_key, not both", domain.ErrInvalid))
		return
	}
	var chosen *domain.Plan
	switch {
	case in.PlanIndex != nil:
		if c.Plan == nil || *in.PlanIndex < 0 || *in.PlanIndex >= len(c.Plan.Plans) {
			a.fail(w, r, fmt.Errorf("%w: plan_index %d: run a plan search first and pick a shown plan", domain.ErrInvalid, *in.PlanIndex))
			return
		}
		chosen = &c.Plan.Plans[*in.PlanIndex]
	case in.MoveKey != "":
		src := c.Plan
		if in.Target != "" {
			src = nil
			if c.Targets != nil {
				if res, ok := c.Targets.Results[domain.NormalizeCountry(in.Target)]; ok {
					src = &res
				}
			}
			if src == nil {
				a.fail(w, r, fmt.Errorf("%w: target %q is not in the latest country comparison", domain.ErrInvalid, in.Target))
				return
			}
		}
		p, err := draftablePlan(src, in.MoveKey)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		chosen = p
	}
	if p := chosen; p != nil {
		ex.Facts, ex.PlanSummary, ex.Requested, ex.Prediction = p.VariantFacts, p.Description, p.Prediction.Origin, p.Prediction
		if p.Verdict != nil {
			ex.Supporting, ex.Counter = p.Verdict.SupportingRuling, p.Verdict.CounterRuling
			if p.RejectionKind == domain.RejectBelowConfidence {
				ex.BelowBar = &export.BelowBar{Confidence: p.Verdict.Confidence, Bar: a.d.PlanConfidence}
			}
		}
	}
	content := export.Markdown(ex)
	now := time.Now().UTC()
	c.ExportedAt, c.UpdatedAt = &now, now
	what := "current setup"
	if chosen != nil {
		what = chosen.Description
		if ex.BelowBar != nil {
			what += fmt.Sprintf(" (below the bar, check %.2f)", ex.BelowBar.Confidence)
		}
	}
	c.Record(store.Event{Kind: "exported", Summary: "draft ruling request for " + what})
	if err := a.d.Cases.Put(r.Context(), c); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "format": "markdown", "content": content})
}

// draftablePlan finds a plan by move key: any shown plan, or a rejected plan
// whose precedent check found support but scored below the bar.
func draftablePlan(res *planner.Result, key string) (*domain.Plan, error) {
	if res == nil {
		return nil, fmt.Errorf("%w: move_key %q: run a plan search first", domain.ErrInvalid, key)
	}
	for i := range res.Plans {
		if res.Plans[i].Move.Key() == key {
			return &res.Plans[i], nil
		}
	}
	for i := range res.Rejected {
		p := &res.Rejected[i]
		if p.Move.Key() != key || key == "" {
			continue
		}
		if p.RejectionKind != domain.RejectBelowConfidence {
			return nil, fmt.Errorf("%w: move_key %q was rejected (%s); only shown plans or plans below the confidence bar can be drafted", domain.ErrInvalid, key, p.RejectionKind)
		}
		return p, nil
	}
	return nil, fmt.Errorf("%w: move_key %q is not in the last plan search", domain.ErrInvalid, key)
}

type rulingDTO struct {
	ID      string  `json:"id"`
	Date    string  `json:"date"`
	Subject string  `json:"subject"`
	Text    string  `json:"text"`
	URL     string  `json:"url"`
	Origin  *string `json:"origin"` // CBP's held origin; null when no single origin
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
		if a.d.Precedents != nil {
			if p, ok := a.d.Precedents.Get(id); ok {
				dto.Origin = nonEmpty(p.Origin)
			}
		}
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
