package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jurisdiction"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/caseagent"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/cross"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/export"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planrule"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/retention"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffshift"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
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

// Estimator estimates a plan search without model calls (planner.Planner,
// D87). A Planner without it has no estimate route.
type Estimator interface {
	Estimate(ctx context.Context, req planner.Request) (planner.Estimate, error)
}

// Clarifier reviews low-confidence predictions (clarify.Clarifier).
type Clarifier interface {
	Review(ctx context.Context, facts string, p domain.Prediction, precedents []domain.Ruling) (clarify.Result, error)
}

// PrecedentStore looks up rulings shown to the engine.
type PrecedentStore interface {
	Get(id string) (domain.Ruling, bool)
}

// PrecedentStores looks a ruling up in each store in turn, so rulings shown
// by an engine with its own pool (the USMCA engine, D61) keep their date,
// subject and held origin in precedents_detail, watch and the ruling view.
type PrecedentStores []PrecedentStore

// Get returns the ruling from the first store that has it.
func (ss PrecedentStores) Get(id string) (domain.Ruling, bool) {
	for _, s := range ss {
		if r, ok := s.Get(id); ok {
			return r, true
		}
	}
	return domain.Ruling{}, false
}

// RulingSource returns a ruling's text (cross.Client).
type RulingSource interface {
	Ruling(ctx context.Context, id string) (cross.Ruling, error)
}

// Deps are the API's collaborators. Router and Planner may be nil (the
// endpoints then degrade: no scope label / 503 for plans).
type Deps struct {
	Engine    Predictor
	Router    Router
	Planner   Planner
	Clarifier Clarifier
	Agent     CaseAgent
	Extractor ProductExtractor
	// MediaExtractor drafts a product from images and scanned PDFs; nil
	// leaves POST /v1/products/extract-media unregistered.
	MediaExtractor MediaExtractor
	Precedents     PrecedentStore
	Rulings        RulingSource
	Cases          store.Cases
	Templates      map[string]planner.Template
	// Jurisdictions are the markets with an origin authority (D680); nil
	// is the United States only.
	Jurisdictions *jurisdiction.Registry
	Summary       func() (evalsummary.Summary, error)
	Demos         []demos.Demo
	FastPathConf  float64
	// FallbackFastPath is the auto-accept bar for a prediction whose
	// trace.model_id is not EngineModel. 0 always reviews (D56).
	FallbackFastPath float64
	// PlanConfidence is the verifier bar for shown plans (named in below-bar drafts).
	PlanConfidence float64
	Provider       string
	EngineModel    string
	RouterModel    string
	VerifierModel  string
	// VerifierPromptVersion is reported in every response's prompt_versions.
	VerifierPromptVersion string
	Logger                *slog.Logger
	// Limits caps model-calling endpoints per client (zero: no limits).
	Limits Limits
	// RateStore, when set, enforces Limits with counters shared by every
	// instance instead of per instance (D70; RATE_LIMIT_STORE=firestore).
	// On a store error a request falls back to this instance's limits.
	RateStore ratelimit.Store
	// RequireBearer refuses requests other than /health without an
	// "Authorization: Bearer" header (REQUIRE_BEARER, off by default; D137).
	RequireBearer bool
	// TrustedProxyCIDRs are the source ranges whose X-Client-IP header may
	// be used for rate-limit keys. Empty means ignore the header and key on
	// RemoteAddr, so a direct caller cannot pick its own bucket (#421).
	TrustedProxyCIDRs []*net.IPNet
	// AdminToken (X-Admin-Token header) bypasses the limits and allows
	// pinning, deleting pinned cases and the batch watch. Empty: no admin.
	AdminToken string
	// Duty, when set, fills duty rates the user did not give from the sourced
	// table, for products with an HTS number (D52).
	Duty *duty.Table
	// LiveMFN, when set (DUTY_LIVE_MFN=on), fills the MFN rate from the
	// USITC HTS for subheadings the table does not cover; the table always
	// wins (D65).
	LiveMFN *duty.Live
	// DutyAdd adds Chapter 99 duties to live MFN rates (Section 301, D90);
	// nil adds none.
	DutyAdd *duty.Adder
	// Now is the clock for exclusion end dates: read once per pricing
	// request, as a UTC instant. Nil: the real clock.
	Now func() time.Time
	// Textile, when set, predicts cases the router labels textile_102_21
	// under 19 CFR 102.21 instead of leaving them out of scope (D51).
	// TextileFastPath is its auto-accept bar; 0 sends every textile case to
	// review.
	Textile         Predictor
	TextileFastPath float64
	// USMCA, when set, predicts cases the router labels usmca_part102 under
	// the Part 102 rules (19 CFR 102.11 / 102.20) instead of leaving them out
	// of scope (D61). USMCAFastPath is its auto-accept bar; 0 sends every
	// USMCA case to review.
	USMCA         Predictor
	USMCAFastPath float64
	// RuleCheck, when set, adds the 19 CFR 102.20 rule check to a case
	// that gives HTS codes (D363). Display only: nothing reads it back.
	RuleCheck *tariffshift.Rules
	// CanadaMarking, when set, serves POST /v1/rules/canada-marking: the
	// SOR/94-23 marking-rule check for goods imported into Canada (D720).
	// A rules check only, never stored and never read by the engine.
	CanadaMarking *tariffshift.Rules
	// EUOrigin and UKOrigin, when set, serve POST /v1/rules/eu-origin and
	// POST /v1/rules/uk-origin: the non-preferential origin list rules of
	// the EU (Annex 22-01) and the UK (Part Four) (D760-D766). Rules checks
	// only, never stored.
	EUOrigin *nporigin.Rules
	UKOrigin *nporigin.Rules
	// Pharma, when set, predicts cases the router labels pharma_api with the
	// pharmaceutical prompt (origin follows the active ingredient) instead of
	// leaving them out of scope (D62). PharmaFastPath is its auto-accept bar;
	// 0 sends every pharmaceutical case to review.
	Pharma         Predictor
	PharmaFastPath float64
	// FamilyBars, when set (FAST_PATH_BY_FAMILY=on), replaces FastPathConf
	// with the bar of the router's subfamily; the triage reason names it
	// (D63). Nil keeps the global bar.
	FamilyBars *triage.FamilyBars
	// Calibration, when set (CALIBRATION=on, the default), attaches the
	// calibrated probability to general-engine predictions it was fitted for
	// and triages those on CalibratedFastPath (D160). Nil: stated confidence
	// only.
	Calibration        *calibrate.Map
	CalibratedFastPath float64
	// CasesListLimit caps GET /v1/cases at every pinned case plus this many
	// others, newest first (0: 200; D58).
	CasesListLimit int
	// Spend, when set, refuses model-calling requests once the day's model
	// spend reaches its cap (D59). The admin token is not exempt from
	// counting, only from the refusal.
	Spend *spend.Meter
	// ExportCFR177 lets export take "format": "cfr177", the ruling-request
	// letter laid out on 19 CFR 177.2 (D66). Off: the field is refused.
	ExportCFR177 bool
	// ATIGACheck serves POST /v1/atiga/check (D927). Off by default.
	ATIGACheck bool
	// CaseOwnership limits each visitor to the pinned demos and their own
	// cases (X-Visitor-ID from the frontend proxy; D58). Off by default.
	CaseOwnership bool
	// StrictAuth (STRICT_AUTH, off by default) makes private case operations
	// need a verified principal: a signed-in user, an API key or the admin
	// token. A bare X-Visitor-ID is not enough (#283).
	StrictAuth bool
	// HTS, when set, serves POST /v1/products/hts-suggest (HTS_SUGGEST=on,
	// D64); nil leaves the route unregistered (404).
	HTS HTSSuggester
	// PartsHTS, when set, serves POST /v1/products/parts-hts-suggest: codes
	// for each part, for the 19 CFR 102.20 rule check (on unless
	// PARTS_HTS_SUGGEST=off, D380); nil leaves the route unregistered (404).
	PartsHTS PartsHTSSuggester
	// Family, when set, serves POST /v1/products/family-check: typed text
	// matched to a template family or "none" (18 Oct).
	Family FamilyChecker
	// CostEstimator, when set, serves POST /v1/products/cost-estimate: AI
	// estimates for blank group costs (#235 step 4).
	CostEstimator CostEstimator
	// BOMVersions serves POST /v1/cases/{id}/bom-version: a newer BOM
	// diffed against the case's product and re-predicted (D67). Off by
	// default (the route is not registered).
	BOMVersions bool
	// BrokerWorkflow serves the review queue, case discussion and sign-offs,
	// and the eRulings filing package (brokerflow.go). Off by default (the
	// routes are not registered).
	BrokerWorkflow bool
	// Teams turns on sign-in and team workspaces (D68): X-User-Token is
	// verified by Users, and Workspaces maps the user to their workspace.
	// Off by default; anonymous requests keep CaseOwnership's rules.
	Teams      bool
	Users      teams.Verifier
	Workspaces teams.Store
	// APIKeys, when set, accepts X-API-Key (its own rate-limit buckets, a
	// daily request quota, cases owned by the key) and serves the admin key
	// endpoints (D69). Nil: the header is ignored and the endpoints are 404.
	APIKeys *apikey.Keys
	// RulingSearch, when set, serves GET /v1/rulings?q= (a text search over
	// the precedent corpus) and the MCP search_rulings tool. Nil: 404.
	RulingSearch RulingSearch
	// MCPOff turns the MCP endpoint (POST /mcp, docs/MCP.md) off. It is on by
	// default wherever the API is.
	MCPOff bool
	// StreamPredict lets POST /v1/cases/predict answer as server-sent events
	// (stage events, then the case) when the client sends Accept:
	// text/event-stream (D71). Off by default: JSON as before.
	StreamPredict bool
	// Checks run on GET /health?deep=1 (store write, model call).
	Checks []HealthCheck
	// Audit, when set, records the audit trail (#284) and serves GET
	// /v1/audit and /v1/audit/verify. Nil: no event is recorded, the routes
	// are not registered and no write is gated on the trail.
	Audit *audit.Service
	// AuditExport serves GET /v1/audit/export (#871) when Audit is set.
	// Off by default.
	AuditExport bool
	// Retention, when set with Teams, serves a workspace's retention policy
	// and tombstones and writes a tombstone before each workspace case
	// deletion (#875). Nil: off.
	Retention retention.Store
}

// maxBody caps request bodies (a product with dozens of parts is ~10 KB).
const maxBody = 256 << 10

type api struct {
	d         Deps
	limiter   *limiter
	shared    *sharedLimiter // nil unless Deps.RateStore is set
	inflight  *inflight      // nil unless Limits.MaxInFlight is set
	authFails *authFailures  // failed API-key checks per client; nil unless Deps.APIKeys is set
	// reviewFails counts invalid broker review tokens per client (D200); nil
	// without limits.
	reviewFails *authFailures
	quotaLocal  *localQuota // API-key requests counted here while the quota store fails
	rulingMu    sync.Mutex
	rulingMemo  map[string]rulingDTO
	askMu       sync.Mutex
	askMemo     map[string]caseagent.Answer
	// flights joins duplicate plan searches (D402); nil runs each request.
	flights *planFlights
}

func (a *api) meta() map[string]any {
	return map[string]any{
		"model_ids":       map[string]string{"engine": a.d.EngineModel, "router": a.d.RouterModel, "verifier": a.d.VerifierModel},
		"prompt_versions": map[string]string{"engine": engine.PromptVersion, "router": router.PromptVersion, "verifier": a.verifierVersion(), "clarify": clarify.PromptVersion, "answers": clarify.AnswersVersion, "case_agent": caseagent.PromptVersion},
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
	case errors.Is(err, apikey.ErrBadRequest):
		return http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, errNeedsVerified):
		return http.StatusUnauthorized, "unauthenticated", needsVerifiedError
	case errors.Is(err, errForbidden):
		return http.StatusForbidden, "forbidden", forbiddenMessage
	case errors.Is(err, errAuditUnavailable):
		return http.StatusServiceUnavailable, "audit_unavailable", "the audit trail is unavailable, so this change was not made; try again"
	case errors.Is(err, errSuperseded), errors.Is(err, errIdempotencyReuse), errors.Is(err, errOriginFactsChanged), errors.Is(err, errFamilyFactsChanged), errors.Is(err, errBrokerConflict):
		return http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, store.ErrNotFound), errors.Is(err, cross.ErrNotFound), errors.Is(err, apikey.ErrNotFound):
		return http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, llm.ErrBadResponse):
		a.d.Logger.ErrorContext(r.Context(), "model bad response", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
		return http.StatusBadGateway, "model_bad_response", "the model returned an unusable answer; try again"
	case errors.Is(err, llm.ErrUnavailable), errors.As(err, &apiErr), errors.Is(err, context.DeadlineExceeded):
		a.d.Logger.ErrorContext(r.Context(), "model unavailable", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
		return http.StatusBadGateway, "model_unavailable", "the model provider is unavailable; try again"
	default:
		a.d.Logger.ErrorContext(r.Context(), "internal error", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
		return http.StatusInternalServerError, "internal", "internal error"
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	// The decoder reads the first JSON value only, as it always did: trailing
	// bytes are never read, so they are not limited either. A tee keeps exactly
	// the bytes the decoder consumed, for explaining a bad operations block.
	var seen bytes.Buffer
	dec := json.NewDecoder(io.TeeReader(http.MaxBytesReader(w, r.Body, maxBody), &seen))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if perr := operationsDecodeError(seen.Bytes()); perr != nil { // a bad operations block says which rule it broke
			return fmt.Errorf("%w; request body: %v", perr, err) // and the outer decode error is kept, never replaced
		}
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
	// Economics are optional money inputs, kept on the case for plan,
	// scenarios and compare-targets calls that omit their own.
	Economics *planner.Economics `json:"economics"`
	// DisplayName optionally names the case on Cases home (as PATCH does).
	DisplayName string `json:"display_name"`
	// Jurisdiction is the market the question is for (D680): us, eu, uk,
	// ca, jp, kr, au, mx or other. Omitted: read from the text, else us.
	Jurisdiction string             `json:"jurisdiction"`
	Japan        *domain.JapanInput `json:"japan"`
}

func (a *api) predict(w http.ResponseWriter, r *http.Request) {
	if !a.mayCreate(w, r) {
		return
	}
	start := time.Now()
	var in predictRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if a.d.StreamPredict && wantsStream(r) {
		a.predictStream(w, r, in, start)
		return
	}
	sc, c, err := a.newCase(r, in)
	if err == nil {
		err = a.auditBegin(r, auditStep{Tenant: tenantOfCase(sc), Object: objectRef("case", sc.ID)})
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	sc, err = a.analyseAndSave(r, sc, c, start, nil)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusCreated, a.caseFields(r, sc))
}

// predictStream runs predict as server-sent events (D71): stage {name:
// router|precedents|engine|clarify, status: started|done, ms} as each step
// starts and finishes (ms since the request began for started, the step's
// duration for done), then done (the same body as the 201 JSON response:
// the case is created) or error {code, message, retryable}. Comments
// (": keep-alive") are sent every keepAlive. Input errors are plain JSON
// responses before the stream opens.
func (a *api) predictStream(w http.ResponseWriter, r *http.Request, in predictRequest, start time.Time) {
	sc, c, err := a.newCase(r, in)
	if err == nil {
		err = a.auditBegin(r, auditStep{Tenant: tenantOfCase(sc), Object: objectRef("case", sc.ID)})
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	s := openStream(w)
	defer s.close()
	st := newStages(start, func(name, status string, ms int64) {
		s.send("stage", map[string]any{"name": name, "status": status, "ms": ms})
	})
	sc, err = a.analyseAndSave(r, sc, c, start, st)
	a.auditStreamEnd(r, err) // before the final event: the stream's headers are already out
	if err != nil {
		status, code, msg := a.errorFor(r, err)
		s.send("error", map[string]any{"code": code, "message": msg, "retryable": status >= 500})
		return
	}
	out := a.meta()
	for k, v := range a.caseFields(r, sc) {
		out[k] = v
	}
	s.send("done", out)
}

// createCase validates a predict request, analyses it and saves the case.
func (a *api) createCase(r *http.Request, in predictRequest, start time.Time) (store.Case, error) {
	sc, c, err := a.newCase(r, in)
	if err != nil {
		return store.Case{}, err
	}
	return a.analyseAndSave(r, sc, c, start, nil)
}

// analyseAndSave analyses a new case, records the prediction and saves it.
// st, when set, is told each step as it runs.
func (a *api) analyseAndSave(r *http.Request, sc store.Case, c domain.Case, start time.Time, st *stages) (store.Case, error) {
	if err := a.analyse(r, &sc, c, st); err != nil {
		return store.Case{}, err
	}
	record(r.Context(), &sc, predictionEvent("predicted", sc, start))
	if err := a.d.Cases.Put(r.Context(), sc); err != nil {
		return store.Case{}, err
	}
	return sc, nil
}

// newCase validates a predict request and builds the case to analyse.
func (a *api) newCase(r *http.Request, in predictRequest) (store.Case, domain.Case, error) {
	facts := strings.TrimSpace(in.Facts)
	market, err := jurisdiction.Resolve(in.Jurisdiction, marketText(facts, in.Product))
	if err != nil {
		return store.Case{}, domain.Case{}, err
	}
	if in.Japan != nil {
		if in.Jurisdiction == "" {
			market = domain.Jurisdiction{Code: jurisdiction.JP, Source: jurisdiction.SourceChosen}
		}
		if market.Code != jurisdiction.JP {
			return store.Case{}, domain.Case{}, fmt.Errorf("%w: japan is only for imports into Japan", domain.ErrInvalid)
		}
	}
	if in.Product != nil {
		if err := in.Product.Validate(); err != nil {
			return store.Case{}, domain.Case{}, err
		}
		if facts == "" && market.Code != jurisdiction.JP {
			tm, ok := a.d.Templates[in.Family]
			if !ok {
				return store.Case{}, domain.Case{}, fmt.Errorf("%w: family %q has no template; known: %s", domain.ErrInvalid, in.Family, a.families())
			}
			facts = planner.Render(*in.Product, tm)
			if in.Jurisdiction == "" {
				market = jurisdiction.Detect(marketText(facts, in.Product))
			}
		}
	}
	for _, id := range in.Exclude {
		if domain.ValidRulingID(id) || (market.Code == jurisdiction.JP && domain.ValidJapanRulingID(id)) {
			continue
		}
		return store.Case{}, domain.Case{}, fmt.Errorf("%w: exclude_rulings: %q is not a ruling number for this market", domain.ErrInvalid, id)
	}
	// Feed facts in the format the engine was benchmarked on: corpus facts
	// files (and rendered products) end with one newline (D20).
	if facts != "" && !strings.HasSuffix(facts, "\n") {
		facts += "\n"
	}
	c := domain.Case{ID: newCaseID(), Date: in.Date.Time, Facts: facts, Product: in.Product, Exclude: in.Exclude, Jurisdiction: &market, Japan: in.Japan}
	if err := c.Validate(); err != nil {
		return store.Case{}, domain.Case{}, err
	}
	if in.Economics != nil {
		if err := validateEconomicsFor(*in.Economics, in.Product, true); err != nil {
			return store.Case{}, domain.Case{}, err
		}
		ec := cleanEconomics(*in.Economics) // an inactive operations block is not stored
		in.Economics = &ec
	}
	displayName := strings.Join(strings.Fields(in.DisplayName), " ")
	if utf8.RuneCountInString(displayName) > MaxDisplayName {
		return store.Case{}, domain.Case{}, fmt.Errorf("%w: display_name is longer than %d characters", domain.ErrInvalid, MaxDisplayName)
	}
	now := time.Now().UTC()
	sc := store.Case{ID: c.ID, Date: in.Date.Format(time.DateOnly), Facts: facts, Family: in.Family, Product: in.Product,
		Exclude: in.Exclude, Jurisdiction: &market, Japan: in.Japan, CurrentOrigin: in.CurrentOrigin, CreatedAt: now, UpdatedAt: now, Economics: in.Economics, DisplayName: displayName,
	}
	sc.Owner, sc.Workspace = a.principalOf(r).newCaseScope() // a signed-in user's (D68), else the credential owner
	return sc, c, nil
}

// ScopeUnavailableReason is the triage reason when a configured scope router
// failed. It does not say the case is out of scope, nor that it is in scope.
const ScopeUnavailableReason = "scope unavailable: the scope router failed, so the legal regime is not confirmed; an expert reviews"

// classify dispatches (router and engine, see dispatch) and decides triage.
// The router labels scope only; it never changes an engine's answer. st, when
// set, is told each step as it runs (nil: not reported); it never changes the
// result.
func (a *api) classify(ctx context.Context, c domain.Case, st *stages) (domain.Prediction, *router.Label, domain.Triage, bool, error) {
	d, err := a.dispatch(ctx, c, st, nil)
	if err != nil {
		return domain.Prediction{}, nil, domain.Triage{}, false, err
	}
	tr, inScope := a.decide(d)
	return d.Result.Prediction, d.Label, tr, inScope, nil
}

// analyse runs router and engine (concurrently), triage and, below the fast
// path, the clarify review; it fills sc. The router and clarify never change
// the engine's answer. st, when set, is told each step as it runs.
func (a *api) analyse(r *http.Request, sc *store.Case, c domain.Case, st *stages) error {
	pred, label, tr, inScope, err := a.classify(r.Context(), c, st)
	if err != nil {
		return err
	}
	sc.Prediction, sc.Router, sc.Triage = pred, label, tr
	sc.Clarify = nil
	// A fallback answer stays on the fallback reason. Clarify would rewrite
	// needs_review into needs_info, and it calls the primary model, which
	// just failed.
	if triage.AnsweredByFallback(pred.Trace, a.d.EngineModel) {
		return nil
	}
	if sc.Triage.Route == domain.RouteNeedsReview && inScope && a.d.Clarifier != nil && a.d.Precedents != nil {
		var ps []domain.Ruling
		for _, id := range pred.Precedents {
			if p, ok := a.d.Precedents.Get(id); ok {
				ps = append(ps, p)
			}
		}
		st.begin("clarify")
		cr, err := a.d.Clarifier.Review(r.Context(), c.Facts, pred, ps)
		st.end("clarify") // a failed review is done too: the case stays needs_review
		if err != nil {
			a.d.Logger.WarnContext(r.Context(), "clarify failed; staying needs_review", "request_id", requestRef(r.Context()), "err", logsafe.Err(err))
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
	start := time.Now()
	sc, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.refuseDemoEdit(w, r, sc) {
		return
	}
	fp := inputFingerprint(sc)
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
	if err := a.auditCase(r, sc, "answers"); err != nil {
		a.fail(w, r, err)
		return
	}
	if sc.OriginalFacts == "" {
		sc.OriginalFacts = sc.Facts
	}
	sc.Answers = append(sc.Answers, in.Answers...)
	facts, _ := clarify.AppendAnswers(sc.OriginalFacts, toClarify(sc.Answers))
	date, _ := time.Parse(time.DateOnly, sc.Date)
	c := domain.Case{ID: sc.ID, Date: date, Facts: facts, Product: sc.Product, Exclude: sc.Exclude, Jurisdiction: caseMarket(sc), Japan: sc.Japan}
	if err := c.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	sc.Facts, sc.UpdatedAt = facts, time.Now().UTC()
	if err := a.analyse(r, &sc, c, nil); err != nil {
		a.fail(w, r, err)
		return
	}
	ev := predictionEvent("answered", sc, start)
	ev.Summary = fmt.Sprintf("%d answer(s) added; ", len(in.Answers)) + ev.Summary
	saved, err := a.d.Cases.Update(r.Context(), sc.ID, func(latest *store.Case) error {
		// New BOM or other input changes while this ran: the answers were
		// appended to old facts, so discard rather than overwrite.
		if err := stillCurrent(latest, fp); err != nil {
			return err
		}
		latest.OriginalFacts, latest.Answers = sc.OriginalFacts, sc.Answers
		latest.Facts, latest.UpdatedAt = sc.Facts, sc.UpdatedAt
		latest.Prediction, latest.Router, latest.Triage, latest.Clarify = sc.Prediction, sc.Router, sc.Triage, sc.Clarify
		// The plan, comparison, watch and draft came from the old facts
		// (every accepted answer adds a fact); scenarios and history stay.
		clearDerived(latest)
		record(r.Context(), latest, ev)
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(r, saved))
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

// rulingRefs is what the corpus knows about each ruling id, for the draft.
func (a *api) rulingRefs(ids []string) map[string]export.RulingRef {
	out := map[string]export.RulingRef{}
	if a.d.Precedents == nil {
		return out
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if p, ok := a.d.Precedents.Get(id); ok {
			out[id] = export.RulingRef{Date: p.Date.Format(time.DateOnly), Subject: p.Subject, Origin: p.Origin}
		}
	}
	return out
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (a *api) caseFields(r *http.Request, c store.Case) map[string]any {
	f := a.caseFieldsBase(r, c)
	// Only a case with broker review links carries the field (D200), so the
	// answer for every other case is byte for byte what it was.
	if rv := a.ownerReviews(r, c); len(rv) > 0 {
		f["broker_reviews"] = rv
	}
	moneyStaleFields(f, c)
	return f
}

func (a *api) caseFieldsBase(r *http.Request, c store.Case) map[string]any {
	// Everything derived (fingerprints, staleness, scenario basis) reads the
	// case as stored; only the predictions shown carry the read-time
	// calibrated value (D180).
	cal := a.withPlanRuleChecks(a.redescribed(a.withCalibration(c)))
	f := map[string]any{
		"id": c.ID, "date": c.Date, "facts": c.Facts, "product": c.Product, "family": c.Family,
		"exclude_rulings": c.Exclude, "current_origin": c.CurrentOrigin, "prediction": cal.Prediction,
		"triage": c.Triage, "router": c.Router, "plan": cal.Plan, "clarify": c.Clarify, "answers": c.Answers, "created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt, "step": c.Step(), "precedents_detail": a.precedentRefs(c.Prediction.Precedents),
		"cited_detail": a.precedentRefs(citedRulingIDs(c.Chat)),
		"scenarios":    a.scenarioFields(c, a.calibratedScenarios(a.currentScenarios(c))), "watch": a.watchFields(c.Watch), "economics": c.Economics, "chat": chatField(c.Chat), "history": a.historyFor(r, c),
		"targets": targetsField(cal.Targets), "pinned": c.Pinned, "economics_priced": a.economicsPriced(c), "display_name": summaryRow(c).DisplayName,
		"can_write": a.canWrite(r, c),
	}
	a.marketFields(f, c)
	if jurisdiction.Of(c.Jurisdiction).Code == jurisdiction.JP {
		f["disclaimer"] = domain.JapanDisclaimer
		f["precedents_detail"] = nil // Japan sources carry their official URLs in japan_preference.
		trace := c.Prediction.Trace
		f["llm_provider"] = trace.Provider
		f["model_ids"] = map[string]string{}
		f["prompt_versions"] = map[string]string{}
		if trace.Identity() != "" {
			f["model_ids"] = map[string]string{"engine": trace.Identity()}
		}
		if trace.PromptVersion != "" {
			f["prompt_versions"] = map[string]string{"engine": trace.PromptVersion}
		}
		if c.Japan != nil {
			f["japan"] = c.Japan
		}
	}
	// What the case's own prediction read (D220), present only when known.
	if src := a.factsSource(c); src != "" {
		f["facts_source"] = src
	}
	// The rule check, only on a case that gives the codes it needs (D363),
	// so every other case reads byte for byte as before.
	if rc := a.ruleCheck(c); rc != nil {
		f["rule_check"] = rc
	}
	// Facts supplied to the EU or UK check (D801), only when there are some.
	if len(c.OriginFacts) > 0 {
		f["origin_facts"] = originFactsOf(c)
	}
	// The family questions (D810), only on a case of a family that has
	// them or with answers saved, so every other case reads byte for byte
	// as before: missing information, answered or known, or why nothing is
	// asked (not modelled).
	if clarify.HasQuestions(c.Family) || c.FamilyFacts != nil {
		f["family_intake"] = a.familyIntakeOf(c)
	}
	if c.FamilyFacts != nil {
		f["family_facts"] = familyFactsOf(c)
	}
	if pb := predictionBeforeAnswers(c); pb != nil {
		f["prediction_before_answers"] = pb
	}
	// CBP changes to rulings the case relies on (D480), present only when
	// there is one. Information only: the stored answer is unchanged.
	if ra := rulingAlertsField(c, alertsNow()); ra != nil {
		f["ruling_alerts"] = ra
	}
	return f
}

func historyField(h []store.Event) []store.Event {
	if h == nil {
		return []store.Event{}
	}
	return h
}

// demoReviewEvents are the history kinds a public demo must not show to a
// non-admin reader: a pinned demo is not shared out for broker review (D200),
// and a link made before the pin is closed by it (issue #447).
var demoReviewEvents = map[string]bool{
	"review_link_created":       true,
	"review_link_revoked":       true,
	"review_link_closed_on_pin": true,
	"broker_decision":           true,
}

// historyFor hides broker-review events on a pinned demo from everyone but
// the admin. The events stay in the stored history; unpinning brings them
// back with the case's other history.
func (a *api) historyFor(r *http.Request, c store.Case) []store.Event {
	h := historyField(c.History)
	if !c.Pinned || a.isAdmin(r) {
		return h
	}
	out := make([]store.Event, 0, len(h))
	for _, e := range h {
		if !demoReviewEvents[e.Kind] {
			out = append(out, e)
		}
	}
	return out
}

// predictionEvent records an analysis with its model and prompt, and the
// usage of the engine, router and clarify calls behind it.
func predictionEvent(kind string, c store.Case, start time.Time) store.Event {
	p := c.Prediction
	var u usage
	u.trace(p.Trace)
	u.label(c.Router)
	if c.Clarify != nil {
		u.trace(c.Clarify.Trace)
	}
	summary := fmt.Sprintf("origin %s, %s, %s", p.Origin, confidencePhrase(p), c.Triage.Route)
	if p.Japan != nil {
		summary = "Japan trade-agreement assessment, " + string(c.Triage.Route)
	}
	return u.on(store.Event{Kind: kind, Summary: summary,
		Provider: p.Trace.Provider, Model: p.Trace.ModelID, Prompt: p.Trace.PromptVersion}, start)
}

// planUsage is the engine and verifier usage of a plan search (cached
// precedent checks cost nothing).
func (a *api) planUsage(u *usage, res planner.Result) {
	u.add(a.d.EngineModel, res.Stats.EngineInTok, res.Stats.EngineOutTok)
	u.add(a.d.VerifierModel, res.Stats.VerifierInTok, res.Stats.VerifierOutTok)
}

func (a *api) planEvent(res planner.Result) store.Event {
	sum := fmt.Sprintf("target %s: %d moves searched, %d shown", res.Target, res.Stats.MovesSearched, len(res.Plans))
	if len(res.Plans) > 0 {
		sum += "; best: " + res.Plans[0].Description
	}
	if res.Stats.VerifierCached > 0 {
		sum += fmt.Sprintf("; %d/%d precedent checks from cache", res.Stats.VerifierCached, res.Stats.VerifierRuns)
	}
	var u usage
	a.planUsage(&u, res)
	e := u.on(store.Event{Kind: "plan_searched", Summary: sum, Provider: res.Baseline.Prediction.Trace.Provider,
		Model: res.Baseline.Prediction.Trace.ModelID, Prompt: res.Baseline.Prediction.Trace.PromptVersion}, time.Time{})
	e.LatencyMS = int64(res.Stats.WallSeconds * 1000)
	return e
}

func chatField(t []store.ChatTurn) []store.ChatTurn {
	if t == nil {
		return []store.ChatTurn{}
	}
	return t
}

// citedRulingIDs is every ruling cited in the conversation, first mention first.
func citedRulingIDs(chat []store.ChatTurn) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, t := range chat {
		for _, id := range t.Cited {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// targetsField is the ranked comparison without the full per-target results,
// which can be large; drafts read them from the store.
func targetsField(t *store.TargetComparison) any {
	if t == nil {
		return nil
	}
	return map[string]any{"checked_at": t.CheckedAt, "ranked": t.Ranked}
}

func (a *api) scenarioFields(c store.Case, ss []store.Scenario) []map[string]any {
	out := make([]map[string]any, len(ss))
	for i, s := range ss {
		out[i] = map[string]any{"label": s.Label, "facts": s.Facts, "product": s.Product, "prediction": s.Prediction,
			"triage": s.Triage, "precedents_detail": a.precedentRefs(s.Prediction.Precedents), "comparison": s.Comparison}
		// The plan search's verdict on the same move (D220), only when it tried it.
		if pc := a.scenarioCheck(c, s); pc != nil {
			out[i]["plan_check"] = pc
		}
	}
	return out
}

func (a *api) watchFields(w *store.Watch) any {
	if w == nil {
		return nil
	}
	out := map[string]any{"checked_at": w.CheckedAt, "status": w.Status, "prediction": w.Prediction,
		"newer_rulings": a.precedentRefs(w.Newer), "precedents_detail": a.precedentRefs(w.Prediction.Precedents)}
	// Provenance (#294), present only on results saved with it.
	for k, v := range map[string]string{"basis": w.Basis, "regime": w.Regime, "original_regime": w.OriginalRegime, "scope": w.Scope, "specialist": w.Specialist} {
		if v != "" {
			out[k] = v
		}
	}
	if w.Triage != nil {
		out["triage"] = w.Triage
	}
	return out
}

// caseSummary is one row of the cases list.
type caseSummary struct {
	ID          string   `json:"id"`
	ProductName string   `json:"product_name"`
	DisplayName string   `json:"display_name"` // the user's name for the case, else product_name
	Date        string   `json:"date"`
	Origin      *string  `json:"predicted_origin"`
	Route       *string  `json:"triage_route"`
	Confidence  *float64 `json:"confidence"`
	// CalibratedConfidence is the prediction's calibrated probability (D160);
	// null when it has none.
	CalibratedConfidence *float64 `json:"calibrated_confidence"`
	Step                 string   `json:"step"`
	UpdatedAt            string   `json:"updated_at"`
	Watch                *string  `json:"watch_status"` // null until re-checked; unchanged | changed
	WatchBasis           *string  `json:"watch_basis"`  // why: store.WatchBasis*; null until re-checked or for a watch saved without provenance
	WatchRoute           *string  `json:"watch_route"`  // the watch result's triage route; null as watch_basis
	Scenarios            int      `json:"scenario_count"`
	Pinned               bool     `json:"pinned"`
	// DemoID is the demo fixture this pinned case was seeded from, when it
	// matches one (#412). It lets the Cases page tell a seeded demo from an
	// unpinned card without guessing from titles.
	DemoID string `json:"demo_id,omitempty"`
	// CanWrite is server-computed for this caller (access.go); nil only in a
	// row built without a request, so absent means unknown and false means read-only.
	CanWrite *bool `json:"can_write,omitempty"`
	// RulingAlerts counts CBP's final and proposed changes to the rulings the
	// case relies on (D480); absent when there are none.
	RulingAlerts *domain.AlertCounts `json:"ruling_alerts,omitempty"`
	// ReviewStatus is the status of the case's newest non-revoked broker
	// review link (#715), as reviewStatus reads it now: an expired link reads
	// expired, never pending. ReviewExpiresAt is that link's expiry, only
	// while it is pending or viewed. Both are absent for a case with no link
	// or only revoked ones. The Cases page's "needs follow-up" filter is
	// display-only and built from these and ruling_alerts.
	ReviewStatus    string     `json:"review_status,omitempty"`
	ReviewExpiresAt *time.Time `json:"review_expires_at,omitempty"`
	// AnswersNote says how supplied answers relate to the origin shown
	// (D814): "rechecked" when a rules-only market's stored check was re-run
	// with them; "added" when answers were saved after a model answer,
	// which is not re-run. Absent otherwise.
	AnswersNote string `json:"answers_note,omitempty"`
}

// listCases lists the cases the caller may see: pinned demos first, then the
// newest limit others. Visibility is applied BEFORE the cap (#301): the
// candidates are read through the caller's own scope, so a busy deployment's
// foreign cases and orphaned health probes can never occupy the slots of the
// caller's own cases. Health probes are excluded before the cap too.
func (a *api) listCases(w http.ResponseWriter, r *http.Request) {
	if a.d.StrictAuth && !a.principalOf(r).Verified() {
		a.fail(w, r, errNeedsVerified) // STRICT_AUTH: the pinned demos are at /v1/demos
		return
	}
	limit := a.d.CasesListLimit
	if limit <= 0 {
		limit = 200
	}
	cs, err := a.candidateCases(r, limit)
	if err == nil {
		cs, err = a.workspaceCases(r.Context(), cs)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	shown := make([]store.Case, 0, len(cs))
	for _, c := range cs {
		if !IsHealthProbeID(c.ID) && a.visible(r, c) {
			shown = append(shown, c)
		}
	}
	shown = store.CapRecent(shown, limit) // visible() is re-checked above; the cap comes last
	rows := make([]caseSummary, 0, len(shown))
	p := a.principalOf(r)
	for _, c := range shown {
		rows = append(rows, a.summaryFor(p, c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": rows})
}

// candidateCases reads the cases the request's principal can possibly see,
// without ever capping a store-wide listing before visibility is known.
// The caller re-checks visible() and caps.
//
//   - Admin sees everything: the global recent window (pinned plus the newest
//     limit others), widened (doubling, no fixed read limit) until orphaned health
//     probes no longer take its slots.
//   - Ownership off, no user, no API key (the old public-demo mode): every
//     anonymous case is meant to be shared, but team and broker cases are
//     not, and there is no owner to query, so this reads the full list and
//     filters. Cost tradeoff: one full scan per listing (O(all cases)); it
//     keeps the documented default correct and is only a dev/demo setting.
//   - Everyone else (visitor, signed-in user, broker key): the pinned demos
//     (one pinned-only read) plus the caller's own owners, each through the
//     store's ListOwner (memory: its own map; Firestore: a full collection
//     read filtered on the decoded owner, see Firestore.ListOwner). A
//     signed-in user's owners are their user hash and the browser's visitor
//     hash; workspace cases are added by workspaceCases. A caller with no
//     owner gets the pinned demos only.
func (a *api) candidateCases(r *http.Request, limit int) ([]store.Case, error) {
	ctx := r.Context()
	p := a.principalOf(r)
	if p.Admin {
		return a.recentWithoutProbes(ctx, limit)
	}
	if !p.ownerScoped(a.d.CaseOwnership) {
		return a.d.Cases.List(ctx)
	}
	cs, err := store.ListRecent(ctx, a.d.Cases, 0) // the pinned demos
	if err != nil {
		return nil, err
	}
	owners := p.listOwners()
	have := make(map[string]bool, len(cs))
	for _, c := range cs {
		have[c.ID] = true
	}
	queried := map[string]bool{"": true}
	for _, o := range owners {
		if queried[o] {
			continue
		}
		queried[o] = true
		mine, err := store.ListOwner(ctx, a.d.Cases, o, limit)
		if err != nil {
			return nil, err
		}
		for _, c := range mine {
			if !have[c.ID] { // an owned pinned case is already in hand
				have[c.ID] = true
				cs = append(cs, c)
			}
		}
	}
	return cs, nil
}

// recentWithoutProbes is the store's recent window with orphaned health
// probes not counted against it. It reads pinned plus the newest n others and
// doubles n until the window holds limit non-probe others or the store has no
// more others than n. Termination and completeness: n grows geometrically, so
// n eventually exceeds the number of cases; the first window with limit real
// others contains the newest limit real others (probes are only dropped), so
// the cap cannot lose or admit a wrong case. There is no fixed read limit:
// with P orphaned probes it takes O(log(P/limit)) reads and at most 2x the
// documents the correct answer needs.
func (a *api) recentWithoutProbes(ctx context.Context, limit int) ([]store.Case, error) {
	for n := limit; ; n *= 2 {
		cs, err := store.ListRecent(ctx, a.d.Cases, n)
		if err != nil {
			return nil, err
		}
		others, probes := 0, 0
		for _, c := range cs {
			if c.Pinned {
				continue
			}
			others++
			if IsHealthProbeID(c.ID) {
				probes++
			}
		}
		if probes == 0 || others < n || others-probes >= limit {
			return cs, nil
		}
	}
}

// summaryRow is a case's row on Cases home.
func summaryRow(c store.Case) caseSummary {
	row := caseSummary{ID: c.ID, Date: c.Date, Step: c.Step(), UpdatedAt: c.UpdatedAt.Format(time.RFC3339), Scenarios: len(c.Scenarios), Pinned: c.Pinned,
		RulingAlerts: alertCounts(c, alertsNow())}
	if rv, ok := newestOpenReview(c.BrokerReviews); ok {
		now := time.Now().UTC()
		row.ReviewStatus = reviewStatus(rv, now)
		if row.ReviewStatus == reviewPending || row.ReviewStatus == reviewViewed {
			exp := rv.ExpiresAt
			row.ReviewExpiresAt = &exp
		}
	}
	if c.Watch != nil {
		row.Watch = &c.Watch.Status
		if c.Watch.Basis != "" {
			row.WatchBasis = &c.Watch.Basis
		}
		if c.Watch.Triage != nil {
			route := string(c.Watch.Triage.Route)
			row.WatchRoute = &route
		}
	}
	if c.Product != nil {
		row.ProductName = c.Product.Name
	} else {
		row.ProductName = firstWords(c.Facts, 8)
	}
	row.DisplayName = row.ProductName
	if c.DisplayName != "" {
		row.DisplayName = c.DisplayName
	}
	row.AnswersNote = answersNote(c)
	if c.Prediction.Origin != "" {
		o, route, conf := c.Prediction.Origin, string(c.Triage.Route), c.Prediction.Confidence
		row.Origin, row.Route, row.Confidence = &o, &route, &conf
		row.CalibratedConfidence = c.Prediction.CalibratedConfidence
	}
	return row
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
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(r, c))
}

type planRequest struct {
	Target string `json:"target"`
	// Jurisdiction, when given, must be the case's market (D680).
	Jurisdiction string                `json:"jurisdiction"`
	Economics    planner.Economics     `json:"economics"`
	ActualChange *planner.ActualChange `json:"actual_change"`
}

func (a *api) plan(w http.ResponseWriter, r *http.Request) {
	if a.d.Planner == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "planner is not configured", nil)
		return
	}
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in planRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	req, st, err := a.planRequestFor(r, c, in, true)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.auditCase(r, c, "plan"); err != nil {
		a.fail(w, r, err)
		return
	}
	if wantsStream(r) {
		a.planStream(w, r, c, req, in.Economics, st)
		return
	}
	idem, err := idempotencyKey(r.Header.Get("Idempotency-Key"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	fp := inputFingerprint(c)
	readOnly := a.demoReadOnly(r, c)
	search := func(ctx context.Context) (map[string]any, error) {
		return a.planSearch(ctx, c, req, in, st, fp, readOnly)
	}
	var body map[string]any
	if a.flights == nil {
		body, err = search(r.Context())
	} else {
		// D402: the same case and inputs join the search already running.
		key := "case:" + c.ID + ":" + planInputs(req, st.basis, fp, readOnly)
		if idem != "" {
			key = "idem:" + c.ID + ":" + idem
		}
		body, _, err = a.flights.do(r.Context(), key, planInputs(req, st.basis, fp, readOnly), idem != "", search)
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, body)
}

// planSearch runs one plan search and stores its result on the case
// (unless the caller may only read a demo). ctx may outlive the request
// that started it (D402).
func (a *api) planSearch(ctx context.Context, c store.Case, req planner.Request, in planRequest, st moneyStamp, fp string, readOnly bool) (map[string]any, error) {
	res, err := a.d.Planner.Plan(ctx, req)
	if err != nil {
		return nil, err
	}
	res = a.calibratedResult(res) // the response and the store carry what a read shows (D251)
	stampBasis(&res, st.basis)
	if !readOnly {
		ev := a.planEvent(res)
		now := time.Now().UTC()
		if _, err := a.d.Cases.Update(ctx, c.ID, func(latest *store.Case) error {
			if err := stillCurrent(latest, fp); err != nil {
				return err
			}
			if err := stillPriced(latest, st, res.EconomicsBasis); err != nil {
				return err
			}
			latest.Plan, latest.UpdatedAt = &res, now
			latest.Economics = economicsFrom(in.Economics, latest.Economics)
			record(ctx, latest, ev)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return map[string]any{"case_id": c.ID, "jurisdiction": jurisdiction.Of(c.Jurisdiction).Code, "plan": a.ruleChecked(c, res)}, nil // the check is added, never stored (D441)
}

// planRequestFor builds the planner request for a case and a plan body.
// fetch false prices duty rates from the table and cached live rates only.
// raw is the hash of the money the request is priced from, taken before the
// tariff table or live lookup fills any rate (see rawBasis).
func (a *api) planRequestFor(r *http.Request, c store.Case, in planRequest, fetch bool) (req planner.Request, st moneyStamp, err error) {
	if err := a.requireMarket(c, in.Jurisdiction); err != nil {
		return planner.Request{}, moneyStamp{}, err
	}
	if c.Product == nil || c.Family == "" {
		return planner.Request{}, moneyStamp{}, fmt.Errorf("%w: plan search needs a case created from a structured product with a family", domain.ErrInvalid)
	}
	date, _ := time.Parse(time.DateOnly, c.Date)
	econ, usedStored, err := effectiveEconomics(in.Economics, c.Economics, c.Product) // omitted: the case's latest
	if err != nil {
		return planner.Request{}, moneyStamp{}, err
	}
	st = moneyStamp{basis: rawBasis(econ), stored: usedStored}
	econ = a.priced(r.Context(), econ, c.Product, append(caseOrigins(c), in.Target), fetch)
	req = planner.Request{
		CaseID: c.ID, Date: planner.Date{Time: date}, Family: c.Family, Target: in.Target,
		Product: *c.Product, Economics: econ, Exclude: c.Exclude, CurrentOrigin: c.CurrentOrigin,
		ActualChange: in.ActualChange, Regime: planRegime(c),
	}
	return req, st, req.Validate()
}

// planEstimate is POST /v1/cases/{id}/plan/estimate: the moves a plan
// search with this body would make, how many are already cached, and a cost
// and wall-time range, with no model call (D87). It has the plan route's
// ownership rules. It takes from the light rate-limit bucket, not the heavy
// one, so checking the estimate does not use up the plan search it is for,
// and the spend cap does not apply: it only reads the caches.
func (a *api) planEstimate(w http.ResponseWriter, r *http.Request) {
	est, ok := a.d.Planner.(Estimator)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "internal", "plan estimates are not configured", nil)
		return
	}
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in planRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	req, _, err := a.planRequestFor(r, c, in, false)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	e, err := est.Estimate(r.Context(), req)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "estimate": e})
}

// economicsPriced is the case's economics with the duty rates a plan search
// would use (sourced table rates filled in, D52; live MFN rates already
// fetched, D65), for display; nil without economics. It never waits on the
// HTS: a live rate shows once a plan, scenarios or compare call fetched it.
func (a *api) economicsPriced(c store.Case) *planner.Economics {
	if c.Economics == nil {
		return nil
	}
	e := a.priced(context.Background(), *c.Economics, c.Product, caseOrigins(c), false)
	return &e
}

// priced fills duty rates the user did not give, for a product with an HTS
// number: from the sourced table when it has the line (D52), else, with
// Deps.LiveMFN, the live MFN rate for each of origins (D65). fetch false
// uses only live rates already cached. Without economics there is nothing
// to price; a failed live lookup leaves the rates as given.
func (a *api) priced(ctx context.Context, e planner.Economics, p *domain.Product, origins []string, fetch bool) planner.Economics {
	if a.d.Duty == nil || p == nil || p.HTS == "" || e.AnnualImportValueUSD <= 0 {
		return e
	}
	if _, err := duty.ParseHTS(p.HTS); err != nil {
		// Malformed: no table, live or auto fill, and say so.
		e.DutyRatesNote = joinNote(e.DutyRatesNote, invalidHTSNote)
		return e
	}
	if _, ok := a.d.Duty.Line(p.HTS); !ok {
		return a.pricedLive(ctx, e, p.HTS, origins, fetch)
	}
	rates, filled := a.d.Duty.Fill(e.DutyRates, p.HTS)
	if len(filled) == 0 {
		return e
	}
	e.DutyRates, e.DutyRatesFromTable = rates, filled
	e.DutyRatesNote = joinNote(e.DutyRatesNote, a.d.Duty.Note(p.HTS, filled))
	return e
}

// pricedLive fills the live MFN rate for the origins the user gave no rate
// for. It asks the HTS only when some origin is missing a rate.
func (a *api) pricedLive(ctx context.Context, e planner.Economics, hts string, origins []string, fetch bool) planner.Economics {
	if a.d.LiveMFN == nil || !missingRate(e, origins) {
		return e
	}
	var m duty.MFN
	var err error
	if fetch {
		m, err = a.d.LiveMFN.MFN(ctx, hts)
	} else {
		var ok bool
		if m, ok, err = a.d.LiveMFN.Cached(hts); !ok {
			return e
		}
	}
	if err != nil {
		if !errors.Is(err, duty.ErrNoRate) {
			a.d.Logger.WarnContext(ctx, "live MFN lookup failed; no rate filled", "request_id", requestRef(ctx), "err", logsafe.Err(err))
		}
		return e
	}
	asOf := a.asOf() // one instant for every origin of this request
	fr := m.FillWithAsOf(e.DutyRates, origins, a.d.DutyAdd, asOf)
	rates, filled, extras, check := fr.Rates, fr.Filled, fr.Extras, fr.Check
	if len(filled) > 0 {
		e.DutyRates, e.DutyRatesLiveMFN = rates, filled
	}
	if len(extras) > 0 {
		e.DutyAdditional = extras
	}
	if a.d.DutyAdd != nil && len(filled) > 0 {
		e.DutyRatesNote = joinNote(e.DutyRatesNote, m.FillNoteWith(filled))
	} else {
		e.DutyRatesNote = joinNote(e.DutyRatesNote, m.FillNote(filled))
	}
	e.DutyRatesNote = joinNote(e.DutyRatesNote, additionalNote(a.d.DutyAdd, extras, check, fr.Expired, asOf))
	return e
}

// asOf is the current UTC instant from the injected clock; a nil clock is
// the real one, so date filtering is never switched off.
func (a *api) asOf() time.Time {
	if a.d.Now == nil {
		return time.Now().UTC()
	}
	return a.d.Now().UTC()
}

// missingRate reports whether any of origins has no duty rate (column 2 and
// US origins never get a live rate, so they do not count).
func missingRate(e planner.Economics, origins []string) bool {
	for _, o := range origins {
		n := domain.DutyCountry(o)
		if _, ok := e.DutyRate(n); n != "" && !ok && !duty.NoLiveRate(n) {
			return true
		}
	}
	return false
}

func joinNote(user, add string) string {
	if user == "" {
		return add
	}
	return user + " " + add
}

// caseOrigins are the countries a case's duty could be owed at: its current
// and predicted origin, and its plants' and components' countries.
func caseOrigins(c store.Case) []string {
	var out []string
	if c.CurrentOrigin != nil {
		out = append(out, c.CurrentOrigin.Country)
	}
	out = append(out, domain.CountryParts(c.Prediction.Origin)...)
	return append(out, productCountries(c.Product)...)
}

func productCountries(p *domain.Product) []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, pl := range p.Plants {
		out = append(out, pl.Country)
	}
	for _, c := range p.Components {
		out = append(out, c.MadeIn)
	}
	return out
}

// economicsFrom keeps the latest economics the user gave (an empty request
// value leaves the stored one).
func economicsFrom(e planner.Economics, old *planner.Economics) *planner.Economics {
	if ownsMoney(e) {
		e = cleanEconomics(e) // an inactive operations block is not stored
		return &e
	}
	return old
}

// planStream runs the plan search as server-sent events: start {target,
// moves}, baseline {plan}, move_started {key}, move_done {key, plan}, then
// done (the same body as the JSON response) or error {code, message,
// retryable}. Comments (": keep-alive") are sent every keepAlive. req carries
// the priced economics the search runs on; raw is what the user sent, and the
// only economics the case stores (as on the JSON path, D56).
func (a *api) planStream(w http.ResponseWriter, r *http.Request, c store.Case, req planner.Request, raw planner.Economics, st moneyStamp) {
	s := openStream(w)
	defer s.close()
	send := s.send
	fp := inputFingerprint(c)
	res, err := a.d.Planner.PlanWithProgress(r.Context(), req, func(e planner.Event) {
		switch e.Kind {
		case planner.EventStarted:
			send("start", map[string]any{"target": e.Target, "moves": e.Moves, "total": e.Total})
		case planner.EventBaseline:
			send("baseline", map[string]any{"plan": a.ruleCheckedPlan(c, req.Target, a.calibratedPlan(e.Plan))})
		case planner.EventMoveStarted:
			send("move_started", map[string]any{"key": e.Move, "done": e.Done, "total": e.Total})
		case planner.EventMoveDone:
			send("move_done", map[string]any{"key": e.Move, "plan": a.ruleCheckedPlan(c, req.Target, a.calibratedPlan(e.Plan)), "done": e.Done, "total": e.Total})
		}
	})
	if err == nil {
		res = a.calibratedResult(res) // D251
		stampBasis(&res, st.basis)
	}
	if err == nil && !a.demoReadOnly(r, c) {
		ev := a.planEvent(res)
		now := time.Now().UTC()
		_, err = a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
			if err := stillCurrent(latest, fp); err != nil {
				return err
			}
			if err := stillPriced(latest, st, res.EconomicsBasis); err != nil {
				return err
			}
			latest.Plan, latest.UpdatedAt = &res, now
			latest.Economics = economicsFrom(raw, latest.Economics)
			record(r.Context(), latest, ev)
			return nil
		})
	}
	a.auditStreamEnd(r, err) // before the final event: the stream's headers are already out
	if err != nil {
		status, code, msg := a.errorFor(r, err)
		send("error", map[string]any{"code": code, "message": msg, "retryable": retryable(status, err)})
		return
	}
	out := a.meta()
	out["case_id"], out["plan"] = c.ID, a.ruleChecked(c, res) // added, never stored (D441)
	out["jurisdiction"] = jurisdiction.Of(c.Jurisdiction).Code
	send("done", out)
}

type exportRequest struct {
	PlanIndex *int `json:"plan_index"`
	// MoveKey picks a plan by move, among shown plans or rejected plans whose
	// precedent check was below the bar.
	MoveKey string `json:"move_key"`
	// PlanID is a plan's plan_id as plan responses carry it (D470): the
	// same value as its move key, accepted under either name.
	PlanID string `json:"plan_id"`
	// Target, with MoveKey, picks from that country's search in the latest
	// country comparison instead of the latest plan search.
	Target string `json:"target"`
	// Preview renders the draft without recording an export: the Draft
	// screen previews on every visit and records only on Download or Print.
	Preview bool `json:"preview"`
	// Format is "markdown" (default, the generic draft) or "cfr177" (the
	// letter laid out on 19 CFR 177.2, when EXPORT_CFR177 is on; D66).
	Format string `json:"format"`
}

// Export formats.
const (
	formatMarkdown = "markdown"
	formatCFR177   = "cfr177"
)

func (a *api) export(w http.ResponseWriter, r *http.Request) {
	c, err := a.loadCase(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.requireMarket(c, ""); err != nil {
		a.fail(w, r, err)
		return
	}
	fp := inputFingerprint(c)
	var in exportRequest
	if r.ContentLength != 0 {
		if err := decode(w, r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	// Only preview:true (a JSON bool, parsed above) is a read. Anything else
	// is the recording branch, which a viewer may not reach (#283).
	if !in.Preview && !a.principalOf(r).allows(c, permEdit) {
		a.fail(w, r, errForbidden)
		return
	}
	d, err := a.buildDraft(r, c, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	format, content, notes, ex, chosen, srcTarget, chosenFrom := d.format, d.content, d.notes, d.ex, d.chosen, d.srcTarget, d.chosenFrom
	// A visitor viewing a pinned demo case gets the draft without writing to
	// it: the seeded demos stay as recorded (D56).
	if in.Preview || (c.Pinned && !a.isAdmin(r)) {
		// A preview of a private case still hands its content to the caller, so
		// the access is recorded (one event, no history and no ExportedAt).
		// A pinned public demo is not private.
		if !a.demoReadOnly(r, c) {
			a.auditSingle(r, auditStep{Tenant: tenantOfCase(c), Object: objectRef("case", c.ID), Detail: format})
		}
		a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "format": format, "content": content, "notes": notes})
		return
	}
	if err := a.auditCase(r, c, format); err != nil {
		a.fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	what := "current setup"
	if chosen != nil {
		what = chosen.Description
		if ex.BelowBar != nil {
			what += fmt.Sprintf(" (below the bar, check %.2f)", ex.BelowBar.Confidence)
		}
	}
	if _, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if err := stillCurrentPlan(latest, fp, srcTarget, chosenFrom); err != nil {
			return err
		}
		latest.ExportedAt, latest.UpdatedAt = &now, now
		summary := "draft ruling request for " + what
		if format == formatCFR177 {
			summary = "19 CFR 177.2 ruling-request letter for " + what
		}
		record(r.Context(), latest, store.Event{Kind: "exported", Summary: summary})
		return nil
	}); err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, map[string]any{"case_id": c.ID, "format": format, "content": content, "notes": notes})
}

// draftResult is a ruling-request draft for a case and a chosen plan (or
// the current setup), with what the recording export needs to check that the
// plan it drew from is still current.
type draftResult struct {
	format, content string
	notes           []string
	ex              export.Input
	chosen          *domain.Plan
	srcTarget       string // the comparison target the plan came from ("" the latest plan search)
	chosenFrom      string // the fingerprint of that plan source
}

// buildDraft renders the draft for c as the export request in asks. It reads
// the case only; export and the broker review link both use it.
func (a *api) buildDraft(r *http.Request, c store.Case, in exportRequest) (draftResult, error) {
	format := strings.ToLower(strings.TrimSpace(in.Format))
	switch {
	case format == "" || format == formatMarkdown:
		format = formatMarkdown
	case format == formatCFR177 && a.d.ExportCFR177:
	case format == formatCFR177:
		return draftResult{}, fmt.Errorf("%w: format cfr177 is not enabled on this server", domain.ErrInvalid)
	default:
		return draftResult{}, fmt.Errorf("%w: format must be markdown or cfr177, got %q", domain.ErrInvalid, in.Format)
	}
	name := ""
	if c.Product != nil {
		name = c.Product.Name
	}
	ex := export.Input{ProductName: name, Product: c.Product, Facts: c.Facts, Requested: c.Prediction.Origin, Prediction: c.Prediction}
	if c.CurrentOrigin != nil {
		ex.CurrentRuled = currentOriginFallback(*c.CurrentOrigin)
	}
	switch id := strings.TrimSpace(in.PlanID); {
	case id == "":
	case in.MoveKey != "" && in.MoveKey != id:
		return draftResult{}, fmt.Errorf("%w: plan_id and move_key name different plans; give one", domain.ErrInvalid)
	default:
		in.MoveKey = id
	}
	if in.PlanIndex != nil && in.MoveKey != "" {
		return draftResult{}, fmt.Errorf("%w: give plan_index or plan_id (move_key), not both", domain.ErrInvalid)
	}
	var chosen *domain.Plan
	var chosenRes *planner.Result // the plan search chosen came from
	// Plans are read as a read shows them: with the 102.20 check on each
	// move and, on a Part 102 case, in the D500 order, so plan_index counts
	// the order the caller saw (D441, D500).
	var latest *planner.Result
	if c.Plan != nil {
		r := a.ruleChecked(c, *c.Plan)
		latest = &r
	}
	switch {
	case in.PlanIndex != nil:
		if latest == nil || *in.PlanIndex < 0 || *in.PlanIndex >= len(latest.Plans) {
			return draftResult{}, fmt.Errorf("%w: plan_index %d: run a plan search first and pick a shown plan (plan_index starts at 0: 0 is the first shown plan)", domain.ErrInvalid, *in.PlanIndex)
		}
		chosen, chosenRes = &latest.Plans[*in.PlanIndex], latest
	case in.MoveKey != "":
		src := latest
		if in.Target != "" {
			src = nil
			if c.Targets != nil {
				if res, ok := c.Targets.Results[domain.NormalizeCountry(in.Target)]; ok {
					res = a.ruleChecked(c, res)
					src = &res
				}
			}
			if src == nil {
				if a.demoReadOnly(r, c) {
					return draftResult{}, fmt.Errorf("%w: target %q is not in this demo's recorded country comparison; start a new case from the demo to compare and draft other countries", domain.ErrInvalid, in.Target)
				}
				return draftResult{}, fmt.Errorf("%w: target %q is not in the latest country comparison; compare it again, then draft", domain.ErrInvalid, in.Target)
			}
		}
		p, err := draftablePlan(src, in.MoveKey)
		if err != nil {
			return draftResult{}, err
		}
		chosen, chosenRes = p, src
	}
	srcTarget := in.Target
	if in.MoveKey == "" {
		srcTarget = "" // plan_index always reads the latest plan search
	}
	chosenFrom := ""
	if chosen != nil {
		chosenFrom = planSourceFingerprint(c, srcTarget)
	}
	if p := chosen; p != nil {
		ex.Facts, ex.PlanSummary, ex.Requested, ex.Prediction = p.VariantFacts, a.planDescription(c, *p, chosenRes), p.Prediction.Origin, p.Prediction
		if c.Product != nil { // the letter lists where things are made after the move
			moved := planner.Apply(*c.Product, p.Move)
			ex.Product = &moved
		}
		ex.RuleCheck = p.RuleCheck
		if p.Verdict != nil {
			ex.Supporting, ex.Counter = p.Verdict.SupportingRuling, p.Verdict.CounterRuling
			if p.RejectionKind == domain.RejectBelowConfidence {
				ex.BelowBar = &export.BelowBar{Confidence: p.Verdict.Confidence, Bar: a.d.PlanConfidence}
			}
		}
	}
	if chosen == nil && a.planRuleOn(c) {
		// The current setup: the same check with no move (D441).
		ex.RuleCheck = planrule.Check(a.d.RuleCheck, *c.Product, domain.Plan{}, "", planRuleApplies(c))
	}
	// Name each cited ruling by date, subject and holding (D220).
	ids := append(append([]string{ex.Supporting, ex.Counter}, ex.Prediction.CitedRulings...), ex.Prediction.Precedents...)
	if c.CurrentOrigin != nil {
		ids = append(ids, c.CurrentOrigin.Ruling)
	}
	ex.Rulings = a.rulingRefs(ids)
	if c.CurrentOrigin != nil && c.CurrentOrigin.Ruling != "" {
		ex.CurrentRuled = strings.TrimSpace(c.CurrentOrigin.Country + " (CBP ruling " + c.CurrentOrigin.Ruling + ")")
		if t, err := time.Parse(time.DateOnly, ex.Rulings[c.CurrentOrigin.Ruling].Date); err == nil {
			ex.CurrentRuled = strings.TrimSpace(c.CurrentOrigin.Country + " (CBP ruling " + c.CurrentOrigin.Ruling + ", " + t.Format("2 January 2006") + ")")
		}
	}
	// The letter's regime is the one that produced the prediction behind the
	// requester's position (the plan's, when one is drafted), read from its
	// stored trace; never the router's label, which a specialist failure
	// leaves pointing at an engine that did not answer.
	regimeNote := ""
	ex.Regime, regimeNote = exportRegime(c, ex.Prediction)
	// A case routed to Part 102 asks the Part 102 marking question even when
	// the general engine made the prediction (a plan searched before D540, or
	// a Part 102 engine that was off or failed); the export
	// package's note says the position is not a Part 102 determination (D520).
	ex.Part102 = ex.Regime == "" && regimeNote == "" && part102Case(c)
	// CBP changes to the rulings this draft relies on go in the notes beside
	// the letter, never in it (#714).
	planTarget := ""
	if chosen != nil {
		planTarget = chosen.Prediction.Origin
	}
	cd, _ := time.Parse(time.DateOnly, c.Date)
	ex.RulingChanges = draftRulingChanges(ex, c.CurrentOrigin, planTarget, cd, alertsNow())
	content := export.Markdown(ex)
	if format == formatCFR177 {
		content = export.CFR177(ex)
	}
	notes := export.Notes(ex, format == formatCFR177)
	if regimeNote != "" {
		notes = append(notes, regimeNote)
	}
	return draftResult{format: format, content: content, notes: notes, ex: ex, chosen: chosen, srcTarget: srcTarget, chosenFrom: chosenFrom}, nil
}

const (
	// exportRegimeUnknownNote: the stored trace names no known engine, so the
	// draft is the general one and says so beside the letter.
	exportRegimeUnknownNote = "Legal framework not confirmed: the engine that produced this prediction is not recorded, so Unde cannot tell which legal framework it was made under. This draft asks the general substantial-transformation question and makes no textile, Part 102 marking or pharmaceutical determination. The broker or counsel must select the appropriate framework before filing. This is a draft aid, not validated legal advice."
	// exportRegimeProvisionalNote: a specialist-labelled case whose prediction
	// (the case's, or a plan's) came from the general engine. Not necessarily
	// a failure: plan searches on textile and pharmaceutical cases use the
	// general engine (D540 gives only Part 102 cases their engine).
	exportRegimeProvisionalNote = "Legal framework not confirmed: this case looks like a %s case, but the prediction behind this draft was made by Unde's general engine, not that framework's engine. (Plan searches use the general engine except on Part 102 cases; the %s engine may also have been unavailable.) This draft asks the general substantial-transformation question and makes no %s determination, so treat the requester's position as provisional. The broker or counsel must select the appropriate framework before filing. This is a draft aid, not validated legal advice."
)

// exportRegimeLabel is a regime in words for the reviewer note.
func exportRegimeLabel(regime string) string {
	switch regime {
	case router.RegimeTextile:
		return "textile (19 CFR 102.21)"
	case router.RegimeUSMCA:
		return "Part 102 marking (19 CFR 102.11)"
	case router.RegimePharma:
		return "pharmaceutical (active ingredient)"
	}
	return regime
}

// exportRegime is the export regime of a prediction and, when the draft
// needs one, a reviewer note for the unknown or provisional-general case. An
// unrecognised trace is rendered as the general draft with a note rather than
// blocked: the letter then makes no specialist claim, and older stored cases
// and demos without a recorded engine stay exportable.
func exportRegime(c store.Case, p domain.Prediction) (regime, note string) {
	switch r := PredictionRegime(p); {
	case r == "":
		return "", exportRegimeUnknownNote
	case r == RegimeGeneral:
		if part102Case(c) {
			return "", "" // asked under Part 102; export.Input.Part102 notes it (D520)
		}
		if c.Router != nil && isSpecialist(c.Router.Regime) {
			return "", fmt.Sprintf(exportRegimeProvisionalNote, exportRegimeLabel(c.Router.Regime), exportRegimeLabel(c.Router.Regime), exportRegimeLabel(c.Router.Regime))
		}
		return "", ""
	default:
		return r, ""
	}
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
	// Superseded is set when a later CBP ruling in the corpus, or a final
	// Customs Bulletin notice, revoked or modified this one (D320, D400).
	Superseded *supersededDTO `json:"superseded,omitempty"`
	// Proposed is set when CBP proposed to revoke or modify it in a fetched
	// Customs Bulletin notice and has not made that final (D400). It is
	// information only and never bars the ruling as a precedent.
	Proposed *proposedDTO `json:"proposed,omitempty"`
}

// supersededDTO names the later ruling that revoked or modified a ruling.
type supersededDTO struct {
	By   string `json:"by"`
	Kind string `json:"kind"` // revoked | modified
	Date string `json:"date"`
	Note string `json:"note"` // e.g. "revoked by H325607 on 2025-08-29"
	// Bulletin and BulletinURL cite the Customs Bulletin notice, when one
	// published or confirmed the change (D400).
	Bulletin    string `json:"bulletin,omitempty"`
	BulletinURL string `json:"bulletin_url,omitempty"`
}

// proposedDTO is a pending proposal to revoke or modify a ruling (D400).
type proposedDTO struct {
	By          string `json:"by"`
	Kind        string `json:"kind"` // revoked | modified
	Bulletin    string `json:"bulletin"`
	BulletinURL string `json:"bulletin_url"`
	Note        string `json:"note"`
}

// supersededAsOf is the supersession of ruling id dated before at (any
// known one when at is zero), or nil.
func supersededAsOf(id string, at time.Time) *supersededDTO {
	s, ok := domain.SupersededAsOf(id, at)
	if !ok {
		return nil
	}
	d := &supersededDTO{By: s.By, Kind: s.Kind, Date: s.Date, Note: s.Note()}
	if s.Bulletin != nil {
		d.Bulletin, d.BulletinURL = s.Bulletin.String(), s.Bulletin.URL
	}
	return d
}

// proposedFor is CBP's pending proposal for ruling id, or nil (D400).
func proposedFor(id string) *proposedDTO {
	p, ok := domain.ProposedFor(id)
	if !ok {
		return nil
	}
	return &proposedDTO{By: p.By, Kind: p.Kind, Bulletin: p.Bulletin.String(), BulletinURL: p.Bulletin.URL, Note: p.Note()}
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
		// Only a fetch from CROSS counts against the lookup limit (D58): a
		// visitor enumerating IDs could get our user agent blocked by CBP,
		// which would break the precedent checks and Ask.
		if a.limiter != nil && !a.isAdmin(r) {
			if allowed, wait := a.allow(r, lookup); !allowed {
				a.tooMany(w, wait)
				return
			}
		}
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
		dto.Superseded = supersededAsOf(id, time.Time{})
		dto.Proposed = proposedFor(id)
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

// demoSummary adds the seeded-case pointer to a demo list row (#412). A demo
// with no pinned_case_id starts a new live case when loaded; one with an id
// opens the recorded case already on this site.
type demoSummary struct {
	demos.Summary
	PinnedCaseID string `json:"pinned_case_id,omitempty"`
}

func (a *api) listDemos(w http.ResponseWriter, r *http.Request) {
	pinned := map[string]string{}
	if cs, err := store.ListRecent(r.Context(), a.d.Cases, 0); err == nil {
		for _, c := range cs {
			if !c.Pinned {
				continue
			}
			name := c.DisplayName
			if name == "" && c.Product != nil {
				name = c.Product.Name
			}
			if id := demoIDFor(a.d.Demos, name); id != "" {
				pinned[id] = c.ID
			}
		}
	}
	rows := make([]demoSummary, 0, len(a.d.Demos))
	for _, s := range demos.Summaries(a.d.Demos) {
		rows = append(rows, demoSummary{Summary: s, PinnedCaseID: pinned[s.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"demos": rows})
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

// verifierVersion is the verifier prompt version in use (D74), verifier-v1
// when the server did not say.
func (a *api) verifierVersion() string {
	if a.d.VerifierPromptVersion != "" {
		return a.d.VerifierPromptVersion
	}
	return verifier.PromptVersion
}

// additionalNote says which Chapter 99 duties were added to live MFN rates,
// which product exclusions the user should check, and which stored ones had
// ended by the as-of date (D90). Exclusions are never applied. An expired one
// is historical metadata only: the note makes no statement about extensions.
func additionalNote(add *duty.Adder, extras map[string][]duty.Extra, check, expired map[string][]duty.Exclusion, asOf time.Time) string {
	if add == nil || (len(extras) == 0 && len(check) == 0 && len(expired) == 0) {
		return ""
	}
	var parts []string
	for _, o := range sortedKeys(extras) {
		var es []string
		for _, x := range extras[o] {
			es = append(es, fmt.Sprintf("%s %s +%g%%", x.Name, x.Heading, math.Round(x.Rate*1000)/10))
		}
		parts = append(parts, o+": "+strings.Join(es, ", "))
	}
	n := ""
	if len(parts) > 0 {
		n = "Additional duties added to the MFN rate (" + add.Source() + "): " + strings.Join(parts, "; ") + "."
	}
	for _, o := range sortedKeys(check) {
		for _, x := range check[o] {
			if x.Status == duty.StatusReview {
				n += fmt.Sprintf(" A product exclusion for %s (%s) has an end date that could not be read: %s. Not applied; confirm its dates before relying on it.", o, x.Heading, strings.TrimSpace(x.Description))
				continue
			}
			until := ""
			if x.EndDate != "" { // conditional exemptions carry no end date
				until = ", until " + x.EndDate
			}
			n += fmt.Sprintf(" A product exclusion may apply for %s (%s%s): %s. Not applied; check whether it covers this product.", o, x.Heading, until, x.Description)
		}
	}
	for _, o := range sortedKeys(expired) {
		for _, x := range expired[o] {
			n += fmt.Sprintf(" Stored product exclusion for %s (%s, %s) is historical, not applicable as of %s per the embedded end date (%s); this makes no statement about any later extension. Not applied.", o, x.Heading, x.Description, asOf.UTC().Format(time.DateOnly), x.EndDate)
		}
	}
	return strings.TrimSpace(n)
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// currentOriginFallback is the caller's statement of CBP's holding on the
// current setup when no ruling date is known: "Country (Ruling)", or just
// "Country" when the optional ruling is absent (#720).
func currentOriginFallback(o planner.KnownOrigin) string {
	if o.Ruling == "" {
		return strings.TrimSpace(o.Country)
	}
	return strings.TrimSpace(o.Country + " (" + o.Ruling + ")")
}
