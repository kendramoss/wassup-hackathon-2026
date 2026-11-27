// Package config reads every tunable from the environment, applies documented
// defaults and validates the result once at startup.
package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

// DefaultFastPath is the auto-accept confidence bar: the lowest confidence at
// which cumulative accuracy stays at or above 95% on dev (95.7%, 81% of cases)
// and held-out (96.7%, 80%) through the proxy (D32).
const DefaultFastPath = 0.92

// DefaultCalibratedFastPath is the auto-accept bar on calibrated probability
// (D160): the lowest step of the calibration map at which in-scope dev
// accuracy is at least 96% on at least 100 cases. It must equal the bar in
// internal/calibrate/map.json (checked by a test). On engine-v1 with Gemini
// 4 Argon it accepts exactly the cases stated confidence 0.92 did.
const DefaultCalibratedFastPath = 0.90

// DefaultPlanBar is the verifier confidence a precedent-supported plan needs
// to be shown; it matches the auto-accept bar (D35).
const DefaultPlanBar = 0.92

// Provider names for LLM_PROVIDER.
const (
	ProviderVertex = "vertex"
	ProviderFake   = "fake"
)

// Case store names for CASES_STORE.
const (
	CasesFirestore = "firestore"
	CasesFile      = "file"
	CasesMemory    = "memory"
)

// Verdict cache stores for VERIFIER_CACHE_STORE.
const (
	VerdictsFirestore = "firestore"
	VerdictsFile      = "file"
	VerdictsOff       = "off"
)

// Rate limiter stores for RATE_LIMIT_STORE (D70).
const (
	RateLocal     = "local"
	RateMemory    = "memory"
	RateFirestore = "firestore"
)

// Config is the validated runtime configuration. Secret fields are excluded
// from String so the config can be logged.
type Config struct {
	Port string

	// LLM provider: vertex (Gemini on Vertex AI with Application Default
	// Credentials), or fake for offline tests and UI work. The LiteLLM proxy
	// used before Vertex access was removed (D49).
	LLMProvider string
	GCPProject  string
	GCPLocation string
	OnCloudRun  bool

	// Warnings are non-fatal production-safety gaps found while loading the
	// config. They are logged at startup (#420) so a deployment cannot look
	// fully locked down just because it boots.
	Warnings []string

	// TrustedProxyCIDRs are the source ranges allowed to set X-Client-IP.
	// Empty means no one is trusted: the backend keys limits on RemoteAddr,
	// so a direct caller cannot choose its own bucket (#421).
	TrustedProxyCIDRs []*net.IPNet

	// Models (IDs depend on the provider; the defaults are served by both).
	EngineModel string
	RouterModel string
	// CaseAgentModel runs the ask-the-case agent; empty uses EngineModel.
	// Flash answers in seconds where Argon's tool turns take 20-30 s each
	// (18 Oct).
	CaseAgentModel string
	VerifierModel  string
	// FallbackEngineModel is called once when the engine's primary model
	// ends in a 429, 503, 504 or deadline (D73). The default is the base
	// model gemini-3.8-flash. FALLBACK_ENGINE_MODEL=off disables it.
	// The fallback client uses FallbackEngineLocation (global, where
	// gemini-3.8-flash is served); the primary client stays on GCPLocation.
	FallbackEngineModel    string
	FallbackEngineLocation string
	// EngineAttempts is how many times the primary engine model is tried
	// before that fallback. 0 means 2 when a fallback is configured; the
	// shared client used by the router, verifier, clarify and agent keeps
	// LLMMaxRetries.
	EngineAttempts int
	// FallbackFastPath is the auto-accept bar for a prediction the fallback
	// model made. 0 (the default) always sends it to review.
	FallbackFastPath float64

	// Model call behaviour.
	LLMTimeout      time.Duration
	LLMMaxRetries   int
	MaxOutputTokens int
	// LLMCallGap is the spacing between model call starts while the shared
	// gate cools down after an overload (429, 5xx or a deadline), for
	// LLMCoolDown. 0 (the default) does not pace (D78, D79).
	LLMCallGap  time.Duration
	LLMCoolDown time.Duration
	// LLMMaxInFlight caps model calls in flight at once across the process
	// (engine, fallback and verifier). 0 = no cap.
	LLMMaxInFlight int

	// Engine, triage and planner tunables.
	PrecedentK           int
	FastPathConfidence   float64
	PlanConfidence       float64
	VerifierMaxToolCalls int
	// VerifierVotes runs the precedent check this many times and keeps the
	// majority; 1 (default) disables voting: 3 votes doubled plan-search time
	// without making verdicts repeatable (D33).
	VerifierVotes int
	// VerifierPanel "on" gives checks with confidence in
	// [VerifierPanelLow, VerifierPanelHigh) two second opinions from different
	// views (skeptic, product) and keeps the majority (D83). Needs prompt v2.
	VerifierPanel                       string
	VerifierPanelLow, VerifierPanelHigh float64
	// VerifierPrompt is v1 or v2 (D74): v2 adds a counter-ruling search and a
	// confidence scale tied to the rulings read.
	VerifierPrompt     string
	PlannerConcurrency int

	// Corpus. The engine's precedent pool is labels + facts files; the
	// verifier searches the local corpus or live CROSS.
	LabelsPath     string
	FactsDir       string
	RawDir         string
	VerifierSearch string // local | cross
	// CasesStore is where cases live: firestore (default on Cloud Run, D25),
	// file (default locally) or memory.
	CasesStore string
	// FirestoreDatabase is the Firestore database ID for CASES_STORE=firestore.
	FirestoreDatabase string
	// CasesDir persists cases as JSON files there; CASES_DIR=off keeps them
	// in memory.
	CasesDir string
	// VerifierCacheDir, when set, stores verifier verdicts so repeated plan
	// searches give the same verdicts (D31); VERIFIER_CACHE_DIR=off disables it.
	VerifierCacheDir string
	// VerifierCacheStore is where verdicts are cached: firestore (default
	// when cases are in Firestore, so all instances share them, D50), file
	// (VerifierCacheDir) or off.
	VerifierCacheStore string
	// RateLightPerMin, RateHeavyPerMin and RateGlobalPerMin cap the
	// model-calling endpoints (D45); 0 disables that limit.
	RateLightPerMin, RateHeavyPerMin, RateGlobalPerMin int
	// RateLookupPerMin caps ruling fetches from CROSS per client (D58).
	RateLookupPerMin int
	// RateLimitStore is where the limits are counted (D70): local (each
	// instance its own buckets), firestore (fixed-window counters shared by
	// all instances; falls back to local on a store error) or memory (the
	// shared limiter in one process, for trying it locally). Unset, it is
	// firestore on Cloud Run when GOOGLE_CLOUD_PROJECT is set, else local
	// (D137). RateLimitStoreDefaulted records that the default chose it.
	RateLimitStore          string
	RateLimitStoreDefaulted bool
	// RateBasicPerMin caps the cheap routes per client (reads, renames,
	// deletes, economics, export, demos, workspaces); 0 disables it (D137).
	RateBasicPerMin int
	// RateMaxInFlight caps how many model-calling requests one client may
	// have running at once on an instance; 0 disables it (D137).
	RateMaxInFlight int
	// RateAuthFailuresPerMin caps invalid API keys per client a minute; 0
	// disables it (D137).
	RateAuthFailuresPerMin int
	// RateReviewPerMin caps the broker review routes (a review link's
	// token) per client a minute; 0 disables it (D200).
	RateReviewPerMin int
	// RateDeclarationPerMin caps the supplier declaration routes (a
	// declaration link's token) per client a minute; 0 disables it (#916).
	RateDeclarationPerMin int
	// DailySpendClientShare is the share of DailySpendCapUSD one client (API
	// key or address) may spend in a UTC day; 0 disables the per-client cap
	// (D137).
	DailySpendClientShare float64
	// RequireBearer (REQUIRE_BEARER on|off, default off) refuses every request
	// but /health that carries no "Authorization: Bearer" header. On Cloud
	// Run the frontend proxy always sends one (D137).
	RequireBearer bool
	// CasesListLimit caps GET /v1/cases: pinned cases plus the newest others (D58).
	CasesListLimit int
	// CaseOwnership (on|off, default off) limits visitors to the pinned demos
	// and their own cases; turn on once the frontend sends X-Visitor-ID (D58).
	CaseOwnership bool
	// StrictAuth (STRICT_AUTH on|off, default off) makes private case
	// operations need a verified principal (signed-in user, API key or admin
	// token): a bare X-Visitor-ID is refused with 401. Pinned demos and the
	// public endpoints stay open. It is not a claim that visitor IDs
	// authenticate anyone (#283).
	StrictAuth bool
	// UsageMetering (USAGE_METERING on|off, default off) counts each
	// workspace's predictions, plan searches and model spend per UTC month
	// and serves workspace budgets with an optional hard cap (#886). Needs
	// TEAMS=on: only signed-in workspace members are metered.
	UsageMetering bool
	// ERPImport (ERP_IMPORT on|off, default off) serves POST
	// /v1/products/erp-import, BOM exports from an ERP filed as cases (#880).
	ERPImport bool
	// Webhooks (WEBHOOKS on|off, default off) lets a workspace admin register
	// signed HTTPS callbacks for workspace events (#915). It needs TEAMS=on
	// and WebhooksSecretKey (a secret, excluded from String, at least 32
	// bytes), which seals endpoint signing secrets at rest.
	Webhooks          bool
	WebhooksSecretKey string
	// Audit (#284). AuditMode is AUDIT: on, off, or auto (the default). Audit is
	// the resolved switch: on, or auto with AUDIT_ACTOR_KEY set. auto never
	// enables the trail without the secret and never generates one; on without
	// it is a startup error. AuditActorKey is a secret (excluded from String).
	AuditMode          string
	Audit              bool
	AuditActorKey      string
	AuditActorKeyID    string
	AuditRetentionDays int
	// AuditStore is firestore, file or memory (default: follows CASES_STORE);
	// AuditDir is the file store directory (AUDIT_DIR=off: memory).
	AuditStore string
	AuditDir   string
	// BOMVersions is on or off (default): on serves POST
	// /v1/cases/{id}/bom-version, a newer BOM diffed against the case's
	// product and re-predicted (D67).
	BOMVersions string
	// AuditExport (on|off, default off) serves GET /v1/audit/export when the
	// audit trail is on (#871).
	AuditExport bool
	// Retention (on|off, default off) serves workspace retention policies and
	// tombstones and lets cmd/retention purge (#875). Its store follows
	// CASES_STORE (file: CASES_DIR/retention.json).
	Retention bool
	// TariffWatch (on|off, default off) records, on each case a plan search
	// prices, the duty basis it was priced on, and serves
	// GET /v1/workspaces/{id}/tariff-watch with the cases cmd/tariffwatch
	// found affected by a newer HTS revision (#917). The route also needs
	// TEAMS=on.
	TariffWatch bool
	// BrokerWorkflow is on or off (default): on serves the review queue,
	// case discussion and sign-offs, and the eRulings filing package
	// (docs/design/broker-workflow.md).
	BrokerWorkflow string
	// Teams (on|off, default off) verifies the Google ID token the frontend
	// proxy forwards as X-User-Token and serves team workspaces (D68).
	// TeamsAudience is the OAuth client ID the tokens must be issued to;
	// required when Teams is on.
	Teams         bool
	TeamsAudience string
	// SSO (on|off, default off) serves enterprise single sign-on (#868),
	// OIDC and SAML, for workspaces with a verified email domain. It needs
	// Teams. SSOSessionKey (a secret, excluded from String, at least 32
	// bytes) signs session tokens and seals stored client secrets;
	// SSOSiteURL is the public https origin whose /sso/... pages receive
	// the identity provider's callbacks; SSOSessionTTL is a session's
	// lifetime (default 8h, at most 24h).
	SSO           bool
	SSOSessionKey string
	SSOSiteURL    string
	SSOSessionTTL time.Duration
	// SCIM (on|off, default off) serves SCIM 2.0 user provisioning (#869)
	// for domains verified on a workspace's SSO connections. It needs SSO.
	SCIM bool
	// AdminConsole (on|off, default off) serves the workspace admin's
	// settings overview (#870). It needs Teams.
	AdminConsole bool
	// SupplierDeclarations (on|off, default off) serves supplier origin
	// declarations (#916): an expiring, revocable link per request, the
	// answers on the case's parts, and the declared origin as a sourced fact.
	// It needs Teams.
	SupplierDeclarations bool
	// APIKeys (on|off, default off) accepts X-API-Key from brokers: the
	// admin issues keys with a daily request quota, each key gets its own
	// rate-limit buckets and owns its cases (D69). Keys are kept in Firestore
	// when cases are, else in memory.
	APIKeys bool
	// DailySpendCapUSD stops new model-calling requests once the UTC day's
	// estimated model spend reaches it (D59); 0 disables the cap.
	DailySpendCapUSD float64
	// AdminToken lets the demo seed bypass the limits, pin demo cases and
	// delete pinned ones. Empty: nobody is admin.
	AdminToken string
	// TextileEngine (on|off, default off) predicts textile cases under
	// 19 CFR 102.21 instead of leaving them out of scope (D51), with
	// precedents from TextileRulings. TextileFastPath is its auto-accept bar;
	// 0 (default) sends every textile case to review.
	TextileEngine   bool
	TextileRulings  string
	TextileFastPath float64
	// TextilePrompt is v1 or v2 (textile-v2: CBP's reasoning shown with each
	// precedent, from TextileReasoning, plus filled made-up guidance; D53).
	TextilePrompt    string
	TextileReasoning string
	// RuleCheck (on|off, default on) adds the deterministic 19 CFR 102.20
	// rule check (internal/tariffshift, D363) to a case's answer when its
	// product gives HTS codes for the good and its parts. Display only: it
	// never changes the prediction, the triage or auto-accept.
	RuleCheck bool
	// JapanEngine (on|off, default on) is the enable gate for the Japan
	// trade-agreement authority. Off (off|false|0) keeps Japan unavailable.
	// On, Japan uses the dedicated JAPAN_LITELLM_* gateway when both are
	// set, else the production Vertex engine model (D710, #694).
	JapanEngine bool
	// USMCAEngine (on|off, default off) predicts cases the router labels
	// usmca_part102 under the Part 102 rules (19 CFR 102.11 / 102.20,
	// usmca-v5 since D602) with precedents from USMCALabels (a copy of
	// eval/data/labels_usmca.jsonl; facts in FactsDir). USMCAFastPath is its
	// auto-accept bar; 0 (default) sends every USMCA case to review (D61).
	USMCAEngine   bool
	USMCALabels   string
	USMCAFastPath float64
	// USMCARulesExt (on|off, default off) extends the USMCA origin-rule check
	// (POST /v1/usmca/check) from the three pilot rows to the rows generated
	// from HTSUS GN 11(o), with de minimis, the parts route and GN 11(p).
	// Off keeps the pilot answers unchanged.
	USMCARulesExt bool
	// PharmaEngine (on|off, default off) predicts pharmaceutical cases (router
	// regime pharma_api) with the pharma-v1 prompt, where origin follows the
	// active ingredient, instead of leaving them out of scope (D62), with
	// precedents from PharmaRulings. PharmaFastPath is its auto-accept bar;
	// 0 (default) sends every pharmaceutical case to review.
	PharmaEngine   bool
	PharmaRulings  string
	PharmaFastPath float64
	// FastPathByFamily (on|off, default off) replaces the global auto-accept
	// bar with the per-subfamily bars fitted on dev by `eval calibrate`
	// (embedded in internal/triage); subfamilies without a fitted bar keep
	// FastPathConfidence (D63).
	FastPathByFamily string
	// Calibration (on|off, default on) attaches the calibrated probability
	// (internal/calibrate, D160) to predictions the map was fitted for, and
	// triages them on CalibratedFastPath instead of FastPathConfidence. Off:
	// stated confidence only, as before D160.
	Calibration        string
	CalibratedFastPath float64
	// HTSSuggest (on|off, default off) serves POST /v1/products/hts-suggest:
	// candidate HTS subheadings from rulings that state one, listed in
	// HTSIndex (written by "corpus hts-index"; D64). Off: the endpoint is a 404.
	HTSSuggest bool
	HTSIndex   string
	// PartsHTSSuggest (on|off, default ON) serves POST
	// /v1/products/parts-hts-suggest: up to 3 codes per part of a product,
	// from the part classifications in PartHTSIndex (written by "corpus
	// part-hts-index") and one model call, rate-limited and spend-capped like
	// every model route (D380). PartsHTSSuggestSet is true when the variable
	// was given: unset, a missing index only turns the route off (with a
	// warning); set to on, it stops the server.
	PartsHTSSuggest    bool
	PartsHTSSuggestSet bool
	PartHTSIndex       string
	// DutyLiveMFN (on|off, default off) looks up the MFN (column 1 general)
	// rate on the USITC HTS for subheadings the sourced duty table does not
	// cover, caching each answer for DutyLiveMFNTTL; each fetch waits at most
	// DutyLiveMFNTimeout (D65).
	DutyLiveMFN        bool
	DutyLiveMFNTTL     time.Duration
	DutyLiveMFNTimeout time.Duration
	// ExportCFR177 (on|off, default off) lets POST /v1/cases/{id}/export
	// take "format": "cfr177", a ruling-request letter laid out on
	// 19 CFR 177.2 (D66). Off: only the Markdown draft.
	ExportCFR177 bool
	exportCFR177 string // as given, checked by validate
	// ATIGACheck (on|off, default off) serves POST /v1/atiga/check and the
	// check_atiga_preference MCP tool: ATIGA preferential origin from a bill
	// of materials, deterministic, no model call (D927).
	ATIGACheck bool
	atigaCheck string // as given, checked by validate
	// AKFTACheck and ACFTACheck (on|off, default off) serve POST
	// /v1/akfta/check and /v1/acfta/check and their MCP tools:
	// ASEAN-Korea and ASEAN-China preferential origin, deterministic, no
	// model call (D949).
	AKFTACheck bool
	akftaCheck string // as given, checked by validate
	ACFTACheck bool
	acftaCheck string // as given, checked by validate
	// ADCVDCheck (on|off, default off) serves POST /v1/adcvd/check, the
	// check_adcvd_exposure MCP tool and adcvd_check on a case: parts that may
	// be subject to a U.S. AD or CVD order, display only, no model call
	// (D939).
	ADCVDCheck bool
	adcvdCheck string // as given, checked by validate
	// ADCVDScopeReader (on|off, default off, needs ADCVD_CHECK=on) makes
	// POST /v1/adcvd/check, and so the MCP tool, have a model read each listed
	// order's written scope against the part's name and drop an order only
	// when the scope clearly points away from the part (D942, D943). One
	// model question per listed order, cached; the case view keeps the plain
	// match. ADCVDReaderModel is the model, gemini-3.8-flash as measured.
	ADCVDScopeReader bool
	adcvdScopeReader string // as given, checked by validate
	ADCVDReaderModel string
	// NearMissAdvice (on|off, default off) adds near-miss advice to the
	// closest plan moves just under the plan bar: what differs from the
	// supporting ruling and what would match it (D81). One model call each,
	// at most two per search; it never changes a verdict.
	NearMissAdvice bool
	nearMissAdvice string // as given, checked by validate
	// FollowAdvice (on|off, default off; needs NearMissAdvice) searches one
	// more move per plan search: the advised near miss plus the groups its
	// advice names (D85).
	FollowAdvice bool
	followAdvice string // as given, checked by validate
	// PlanMaxMoves caps the moves a plan search lists; three-group moves are
	// added only while the total stays within it (D84). Default 12.
	PlanMaxMoves int
	// StreamPredict (on|off, default off) lets POST /v1/cases/predict answer
	// as server-sent stage events when asked for text/event-stream (D71).
	StreamPredict string
	// EvalSlice (on|off, default off) lets `eval slice` spend model calls on
	// the 30-case regression slice (D72); the comparison against the
	// committed baseline (-in FILE) needs no model and works either way.
	// EvalSliceMaxDrop is how many fewer correct cases than the baseline
	// still pass (default 2); EvalSliceBandMin is the accuracy the cases at
	// confidence >= 0.95 must keep (default 0.95).
	EvalSlice        bool
	EvalSliceMaxDrop int
	// EvalSliceMaxErrors is how many slice cases may fail (provider errors)
	// before the run is inconclusive instead of failed (default 3, D88).
	EvalSliceMaxErrors int
	EvalSliceBandMin   float64
}

// StreamPredictOn reports whether STREAM_PREDICT is on.
func (c Config) StreamPredictOn() bool { return c.StreamPredict == "on" }

// Load reads the configuration from the process environment.
func Load() (Config, error) { return FromEnv(os.Getenv) }

// FromEnv reads the configuration through getenv, so tests can supply a map.
func FromEnv(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	c := Config{
		Port:                   e.str("PORT", "8080"),
		LLMProvider:            strings.ToLower(e.str("LLM_PROVIDER", ProviderVertex)),
		GCPProject:             e.str("GOOGLE_CLOUD_PROJECT", ""),
		GCPLocation:            e.str("GOOGLE_CLOUD_LOCATION", "global"),
		OnCloudRun:             e.str("K_SERVICE", "") != "",
		EngineModel:            e.str("ENGINE_MODEL", "gemini-4-argon"),
		RouterModel:            e.str("ROUTER_MODEL", "gemini-3.8-flash"),
		CaseAgentModel:         e.str("CASE_AGENT_MODEL", ""),
		VerifierModel:          e.str("VERIFIER_MODEL", "gemini-4-argon"),
		FallbackEngineModel:    fallbackModel(e.str("FALLBACK_ENGINE_MODEL", "gemini-3.8-flash")),
		FallbackEngineLocation: e.str("FALLBACK_ENGINE_LOCATION", "global"),
		EngineAttempts:         e.integer("ENGINE_ATTEMPTS", 0),
		FallbackFastPath:       e.float("FALLBACK_FAST_PATH", 0),
		LLMTimeout:             e.duration("LLM_TIMEOUT", 30*time.Second), // per attempt; 4 attempts fit Cloud Run's 300 s (D54)
		LLMMaxRetries:          e.integer("LLM_MAX_RETRIES", 3),
		LLMCallGap:             e.duration("LLM_CALL_GAP", 0),
		LLMCoolDown:            e.duration("LLM_COOL_DOWN", 60*time.Second),
		LLMMaxInFlight:         e.integer("LLM_MAX_IN_FLIGHT", 0),
		MaxOutputTokens:        e.integer("LLM_MAX_OUTPUT_TOKENS", 8000),
		PrecedentK:             e.integer("PRECEDENT_K", 5),
		FastPathConfidence:     e.float("FAST_PATH_CONFIDENCE", DefaultFastPath),
		PlanConfidence:         e.float("PLAN_CONFIDENCE", DefaultPlanBar),
		VerifierMaxToolCalls:   e.integer("VERIFIER_MAX_TOOL_CALLS", 6),
		VerifierVotes:          e.integer("VERIFIER_VOTES", 1),
		VerifierPanel:          strings.ToLower(e.str("VERIFIER_PANEL", "off")),
		VerifierPanelLow:       e.float("VERIFIER_PANEL_LOW", 0.89),
		VerifierPanelHigh:      e.float("VERIFIER_PANEL_HIGH", 0.95),
		VerifierPrompt:         strings.ToLower(e.str("VERIFIER_PROMPT", "v2")),
		PlannerConcurrency:     e.integer("PLANNER_CONCURRENCY", 8),
		// Defaults resolve both from backend/ (local) and from / in the image.
		LabelsPath:             e.str("LABELS_PATH", "data/labels.jsonl"),
		FactsDir:               e.str("CORPUS_FACTS_DIR", "data/facts"),
		RawDir:                 e.str("CORPUS_RAW_DIR", "../data/corpus/raw"),
		VerifierSearch:         strings.ToLower(e.str("VERIFIER_SEARCH", "cross")),
		CasesDir:               e.dir("CASES_DIR", "../data/cases"),
		FirestoreDatabase:      e.str("FIRESTORE_DATABASE", "(default)"),
		VerifierCacheDir:       e.dir("VERIFIER_CACHE_DIR", "../data/verifier-cache"),
		RateLightPerMin:        e.integer("RATE_LIGHT_PER_MIN", 20),
		RateHeavyPerMin:        e.integer("RATE_HEAVY_PER_MIN", 4),
		RateGlobalPerMin:       e.integer("RATE_GLOBAL_PER_MIN", 120),
		RateLookupPerMin:       e.integer("RATE_LOOKUP_PER_MIN", 30),
		RateBasicPerMin:        e.integer("RATE_BASIC_PER_MIN", 120),
		RateMaxInFlight:        e.integer("RATE_MAX_IN_FLIGHT", 4),
		RateAuthFailuresPerMin: e.integer("RATE_AUTH_FAILURES_PER_MIN", 10),
		RateReviewPerMin:       e.integer("RATE_REVIEW_PER_MIN", 30),
		RateDeclarationPerMin:  e.integer("RATE_DECLARATION_PER_MIN", 30),
		CasesListLimit:         e.integer("CASES_LIST_LIMIT", 200),
		BOMVersions:            strings.ToLower(e.str("BOM_VERSIONS", "off")),
		BrokerWorkflow:         strings.ToLower(e.str("BROKER_WORKFLOW", "off")),
		DailySpendCapUSD:       e.float("DAILY_SPEND_CAP_USD", 0),
		DailySpendClientShare:  e.float("DAILY_SPEND_CLIENT_SHARE", 0.1),
		AdminToken:             e.str("ADMIN_TOKEN", ""),
		AuditActorKey:          e.str("AUDIT_ACTOR_KEY", ""),
		AuditActorKeyID:        e.str("AUDIT_ACTOR_KEY_ID", "1"),
		AuditRetentionDays:     e.integer("AUDIT_RETENTION_DAYS", 90),
		AuditDir:               e.dir("AUDIT_DIR", "../data/audit"),
		TeamsAudience:          e.str("TEAMS_AUDIENCE", ""),
		SSOSessionKey:          e.str("SSO_SESSION_KEY", ""),
		WebhooksSecretKey:      e.str("WEBHOOKS_SECRET_KEY", ""),
		SSOSiteURL:             e.str("SSO_SITE_URL", ""),
		SSOSessionTTL:          e.duration("SSO_SESSION_TTL", 8*time.Hour),
		TextileRulings:         e.str("TEXTILE_RULINGS", "data/textile_rulings.json"),
		TextileFastPath:        e.float("TEXTILE_FAST_PATH", 0),
		TextilePrompt:          strings.ToLower(e.str("TEXTILE_PROMPT", "v1")),
		TextileReasoning:       e.str("TEXTILE_REASONING", "data/textile_reasoning.json"),
		USMCALabels:            e.str("USMCA_LABELS", "data/labels_usmca.jsonl"),
		USMCAFastPath:          e.float("USMCA_FAST_PATH", 0),
		PharmaRulings:          e.str("PHARMA_RULINGS", "data/pharma_rulings.json"),
		PharmaFastPath:         e.float("PHARMA_FAST_PATH", 0),
		FastPathByFamily:       strings.ToLower(e.str("FAST_PATH_BY_FAMILY", "off")),
		Calibration:            strings.ToLower(e.str("CALIBRATION", "on")),
		CalibratedFastPath:     e.float("CALIBRATED_FAST_PATH", DefaultCalibratedFastPath),
		HTSIndex:               e.str("HTS_INDEX", "data/hts_index.json"),
		PartHTSIndex:           e.str("PART_HTS_INDEX", "data/part_hts_index.json"),
		PartsHTSSuggestSet:     strings.TrimSpace(e.get("PARTS_HTS_SUGGEST")) != "",
		DutyLiveMFNTTL:         e.duration("DUTY_LIVE_MFN_TTL", 24*time.Hour),
		DutyLiveMFNTimeout:     e.duration("DUTY_LIVE_MFN_TIMEOUT", 5*time.Second),
		EvalSliceMaxDrop:       e.integer("EVAL_SLICE_MAX_DROP", 2),
		EvalSliceMaxErrors:     e.integer("EVAL_SLICE_MAX_ERRORS", 3),
		EvalSliceBandMin:       e.float("EVAL_SLICE_BAND_MIN", 0.95),
	}
	switch v := strings.ToLower(e.str("CASE_OWNERSHIP", "off")); v {
	case "on", "true", "1":
		c.CaseOwnership = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("CASE_OWNERSHIP must be on or off, got %q", v))
	}
	for _, f := range []struct {
		key string
		dst *bool
	}{{"USAGE_METERING", &c.UsageMetering}, {"ERP_IMPORT", &c.ERPImport}, {"WEBHOOKS", &c.Webhooks}} {
		switch v := strings.ToLower(e.str(f.key, "off")); v {
		case "on", "true", "1":
			*f.dst = true
		case "off", "false", "0":
		default:
			e.errs = append(e.errs, fmt.Errorf("%s must be on or off, got %q", f.key, v))
		}
	}
	switch v := strings.ToLower(e.str("STRICT_AUTH", "off")); v {
	case "on", "true", "1":
		c.StrictAuth = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("STRICT_AUTH must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("AUDIT", "auto")); v {
	case "on", "true", "1":
		c.AuditMode = "on"
	case "off", "false", "0":
		c.AuditMode = "off"
	case "auto":
		c.AuditMode = "auto"
	default:
		e.errs = append(e.errs, fmt.Errorf("AUDIT must be on, off or auto, got %q", v))
	}
	c.Audit = c.AuditMode == "on" || (c.AuditMode == "auto" && c.AuditActorKey != "")
	switch v := strings.ToLower(e.str("REQUIRE_BEARER", "off")); v {
	case "on", "true", "1":
		c.RequireBearer = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("REQUIRE_BEARER must be on or off, got %q", v))
	}
	// On Cloud Run, an unset RATE_LIMIT_STORE shares the limits in Firestore:
	// per-instance buckets let a client through about once per instance
	// (D57, D70, D137). Without a project it stays local.
	defRate := RateLocal
	if c.OnCloudRun && c.GCPProject != "" && e.str("RATE_LIMIT_STORE", "") == "" {
		defRate, c.RateLimitStoreDefaulted = RateFirestore, true
	}
	c.RateLimitStore = strings.ToLower(e.str("RATE_LIMIT_STORE", defRate))
	// X-Client-IP is only meaningful when it was set by a proxy we control.
	// A comma-separated CIDR list keeps the trust decision explicit (#421).
	cidrs, err := parseCIDRs(e.str("TRUSTED_PROXY_CIDRS", ""))
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err))
	}
	c.TrustedProxyCIDRs = cidrs
	switch v := strings.ToLower(e.str("TEAMS", "off")); v {
	case "on", "true", "1":
		c.Teams = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("TEAMS must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("SSO", "off")); v {
	case "on", "true", "1":
		c.SSO = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("SSO must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("SCIM", "off")); v {
	case "on", "true", "1":
		c.SCIM = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("SCIM must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("ADMIN_CONSOLE", "off")); v {
	case "on", "true", "1":
		c.AdminConsole = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("ADMIN_CONSOLE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("SUPPLIER_DECLARATIONS", "off")); v {
	case "on", "true", "1":
		c.SupplierDeclarations = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("SUPPLIER_DECLARATIONS must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("API_KEYS", "off")); v {
	case "on", "true", "1":
		c.APIKeys = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("API_KEYS must be on or off, got %q", v))
	}
	for name, dst := range map[string]*bool{"AUDIT_EXPORT": &c.AuditExport, "RETENTION": &c.Retention, "TARIFF_WATCH": &c.TariffWatch} {
		switch v := strings.ToLower(e.str(name, "off")); v {
		case "on", "true", "1":
			*dst = true
		case "off", "false", "0":
		default:
			e.errs = append(e.errs, fmt.Errorf("%s must be on or off, got %q", name, v))
		}
	}
	switch v := strings.ToLower(e.str("TEXTILE_ENGINE", "off")); v {
	case "on", "true", "1":
		c.TextileEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("TEXTILE_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("JAPAN_ENGINE", "on")); v {
	case "on", "true", "1":
		c.JapanEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("JAPAN_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("RULE_CHECK", "on")); v {
	case "on", "true", "1":
		c.RuleCheck = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("RULE_CHECK must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("USMCA_ENGINE", "off")); v {
	case "on", "true", "1":
		c.USMCAEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("USMCA_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("USMCA_RULES_EXT", "off")); v {
	case "on", "true", "1":
		c.USMCARulesExt = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("USMCA_RULES_EXT must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("PHARMA_ENGINE", "off")); v {
	case "on", "true", "1":
		c.PharmaEngine = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("PHARMA_ENGINE must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("HTS_SUGGEST", "off")); v {
	case "on", "true", "1":
		c.HTSSuggest = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("HTS_SUGGEST must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("PARTS_HTS_SUGGEST", "on")); v {
	case "on", "true", "1":
		c.PartsHTSSuggest = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("PARTS_HTS_SUGGEST must be on or off, got %q", v))
	}
	switch v := strings.ToLower(e.str("DUTY_LIVE_MFN", "off")); v {
	case "on", "true", "1":
		c.DutyLiveMFN = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("DUTY_LIVE_MFN must be on or off, got %q", v))
	}
	c.nearMissAdvice = strings.ToLower(e.str("NEAR_MISS_ADVICE", "off"))
	c.NearMissAdvice = c.nearMissAdvice == "on" || c.nearMissAdvice == "true" || c.nearMissAdvice == "1"
	c.followAdvice = strings.ToLower(e.str("FOLLOW_ADVICE", "off"))
	c.FollowAdvice = c.followAdvice == "on" || c.followAdvice == "true" || c.followAdvice == "1"
	c.PlanMaxMoves = e.integer("PLAN_MAX_MOVES", 12)
	c.exportCFR177 = strings.ToLower(e.str("EXPORT_CFR177", "off"))
	c.ExportCFR177 = c.exportCFR177 == "on" || c.exportCFR177 == "true" || c.exportCFR177 == "1"
	c.atigaCheck = strings.ToLower(e.str("ATIGA_CHECK", "off"))
	c.ATIGACheck = c.atigaCheck == "on" || c.atigaCheck == "true" || c.atigaCheck == "1"
	c.akftaCheck = strings.ToLower(e.str("AKFTA_CHECK", "off"))
	c.AKFTACheck = c.akftaCheck == "on" || c.akftaCheck == "true" || c.akftaCheck == "1"
	c.acftaCheck = strings.ToLower(e.str("ACFTA_CHECK", "off"))
	c.ACFTACheck = c.acftaCheck == "on" || c.acftaCheck == "true" || c.acftaCheck == "1"
	c.adcvdCheck = strings.ToLower(e.str("ADCVD_CHECK", "off"))
	c.ADCVDCheck = c.adcvdCheck == "on" || c.adcvdCheck == "true" || c.adcvdCheck == "1"
	c.adcvdScopeReader = strings.ToLower(e.str("ADCVD_SCOPE_READER", "off"))
	c.ADCVDScopeReader = c.adcvdScopeReader == "on" || c.adcvdScopeReader == "true" || c.adcvdScopeReader == "1"
	c.ADCVDReaderModel = e.str("ADCVD_READER_MODEL", "gemini-3.8-flash")
	switch v := strings.ToLower(e.str("STREAM_PREDICT", "off")); v {
	case "on", "true", "1":
		c.StreamPredict = "on"
	case "off", "false", "0":
		c.StreamPredict = "off"
	default:
		c.StreamPredict = v // refused by validate
	}
	switch v := strings.ToLower(e.str("EVAL_SLICE", "off")); v {
	case "on", "true", "1":
		c.EvalSlice = true
	case "off", "false", "0":
	default:
		e.errs = append(e.errs, fmt.Errorf("EVAL_SLICE must be on or off, got %q", v))
	}
	defStore := CasesFile
	if c.OnCloudRun {
		defStore = CasesFirestore // local disk is lost on restart there
	}
	c.CasesStore = strings.ToLower(e.str("CASES_STORE", defStore))
	defCache := VerdictsFile
	switch {
	case c.VerifierCacheDir == "": // VERIFIER_CACHE_DIR=off
		defCache = VerdictsOff
	case c.CasesStore == CasesFirestore:
		defCache = VerdictsFirestore
	}
	c.VerifierCacheStore = strings.ToLower(e.str("VERIFIER_CACHE_STORE", defCache))
	defAudit := c.CasesStore
	if defAudit == CasesFile && c.AuditDir == "" {
		defAudit = CasesMemory // AUDIT_DIR=off
	}
	c.AuditStore = strings.ToLower(e.str("AUDIT_STORE", defAudit))
	// Two attempts, then the fallback: 4 attempts × 30 s plus backoff can
	// take about two minutes before any other model is asked (D56).
	if c.FallbackEngineModel != "" && c.EngineAttempts == 0 {
		c.EngineAttempts = 2
	}
	if err := errors.Join(append(e.errs, c.validate())...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	c.Warnings = c.productionWarnings()
	return c, nil
}

// productionWarnings names the auth/audit flags that are deliberately off in
// a Cloud Run deployment. They stay warnings, not errors: the public demo
// needs anonymous visitors and the owner must verify header pass-through
// before turning on REQUIRE_BEARER. Hiding the gap is what is unsafe (#420).
func (c Config) productionWarnings() []string {
	if !c.OnCloudRun {
		return nil
	}
	var out []string
	if !c.StrictAuth {
		out = append(out, "STRICT_AUTH is off: a visitor ID alone is accepted as an anonymous principal; Cloud Run IAM and CASE_OWNERSHIP remain the boundaries")
	}
	if !c.RequireBearer {
		out = append(out, "REQUIRE_BEARER is off: the backend accepts proxied requests without a bearer token; verify Cloud Run header pass-through before enabling it")
	}
	if c.AuditActorKey == "" || !c.Audit {
		out = append(out, "AUDIT_ACTOR_KEY is unset (or AUDIT=off): no audit trail is written; this is an explicit product choice, not certification")
	}
	return out
}

func (c Config) validate() error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	// Defense in depth: the env.float parser rejects nonfinite values, but a
	// Config built directly must not carry them either (all comparisons with
	// NaN are false, so range checks alone cannot catch it).
	for name, v := range map[string]float64{
		"FALLBACK_FAST_PATH":       c.FallbackFastPath,
		"FAST_PATH_CONFIDENCE":     c.FastPathConfidence,
		"CALIBRATED_FAST_PATH":     c.CalibratedFastPath,
		"PLAN_CONFIDENCE":          c.PlanConfidence,
		"VERIFIER_PANEL_LOW":       c.VerifierPanelLow,
		"VERIFIER_PANEL_HIGH":      c.VerifierPanelHigh,
		"DAILY_SPEND_CAP_USD":      c.DailySpendCapUSD,
		"DAILY_SPEND_CLIENT_SHARE": c.DailySpendClientShare,
		"TEXTILE_FAST_PATH":        c.TextileFastPath,
		"USMCA_FAST_PATH":          c.USMCAFastPath,
		"PHARMA_FAST_PATH":         c.PharmaFastPath,
		"EVAL_SLICE_BAND_MIN":      c.EvalSliceBandMin,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			bad("%s must be a finite number, got %v", name, v)
		}
	}
	if c.VerifierPrompt != "v1" && c.VerifierPrompt != "v2" && c.VerifierPrompt != "v3" {
		bad("VERIFIER_PROMPT must be v1, v2 or v3, got %q", c.VerifierPrompt)
	}
	switch {
	case c.VerifierPanel != "on" && c.VerifierPanel != "off":
		bad("VERIFIER_PANEL must be on or off, got %q", c.VerifierPanel)
	case c.VerifierPanel == "on" && c.VerifierPrompt != "v2":
		bad("VERIFIER_PANEL=on needs VERIFIER_PROMPT=v2, got %q", c.VerifierPrompt)
	case c.VerifierPanelLow < 0 || c.VerifierPanelHigh > 1 || c.VerifierPanelLow >= c.VerifierPanelHigh:
		bad("VERIFIER_PANEL_LOW must be below VERIFIER_PANEL_HIGH, both in [0, 1], got %v and %v", c.VerifierPanelLow, c.VerifierPanelHigh)
	}
	switch c.LLMProvider {
	case ProviderVertex:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when LLM_PROVIDER=vertex")
		}
	case ProviderFake:
		if c.OnCloudRun {
			bad("LLM_PROVIDER=fake cannot run on Cloud Run")
		}
	default:
		bad("LLM_PROVIDER must be vertex or fake, got %q (the LiteLLM proxy was removed, D49)", c.LLMProvider)
	}
	for name, v := range map[string]string{"ENGINE_MODEL": c.EngineModel, "ROUTER_MODEL": c.RouterModel, "VERIFIER_MODEL": c.VerifierModel} {
		if v == "" {
			bad("%s must not be empty", name)
		}
	}
	for name, v := range map[string]float64{"FAST_PATH_CONFIDENCE": c.FastPathConfidence, "CALIBRATED_FAST_PATH": c.CalibratedFastPath, "PLAN_CONFIDENCE": c.PlanConfidence} {
		if v <= 0 || v > 1 {
			bad("%s must be in (0, 1], got %v", name, v)
		}
	}
	positive := map[string]int{"PRECEDENT_K": c.PrecedentK, "VERIFIER_MAX_TOOL_CALLS": c.VerifierMaxToolCalls, "VERIFIER_VOTES": c.VerifierVotes,
		"PLANNER_CONCURRENCY": c.PlannerConcurrency, "LLM_MAX_OUTPUT_TOKENS": c.MaxOutputTokens, "PLAN_MAX_MOVES": c.PlanMaxMoves}
	for name, v := range positive {
		if v <= 0 {
			bad("%s must be positive, got %d", name, v)
		}
	}
	if c.DailySpendClientShare < 0 || c.DailySpendClientShare > 1 {
		bad("DAILY_SPEND_CLIENT_SHARE must be 0 (off) or in (0, 1], got %v", c.DailySpendClientShare)
	}
	if c.DailySpendCapUSD < 0 {
		bad("DAILY_SPEND_CAP_USD must be 0 (no cap) or positive, got %v", c.DailySpendCapUSD)
	}
	if c.CasesListLimit < 1 {
		bad("CASES_LIST_LIMIT must be positive, got %d", c.CasesListLimit)
	}
	switch c.exportCFR177 {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("EXPORT_CFR177 must be on or off, got %q", c.exportCFR177)
	}
	switch c.adcvdCheck {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("ADCVD_CHECK must be on or off, got %q", c.adcvdCheck)
	}
	switch c.adcvdScopeReader {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("ADCVD_SCOPE_READER must be on or off, got %q", c.adcvdScopeReader)
	}
	if c.ADCVDScopeReader && !c.ADCVDCheck {
		bad("ADCVD_SCOPE_READER=on needs ADCVD_CHECK=on")
	}
	switch c.atigaCheck {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("ATIGA_CHECK must be on or off, got %q", c.atigaCheck)
	}
	switch c.akftaCheck {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("AKFTA_CHECK must be on or off, got %q", c.akftaCheck)
	}
	switch c.acftaCheck {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("ACFTA_CHECK must be on or off, got %q", c.acftaCheck)
	}
	switch c.nearMissAdvice {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("NEAR_MISS_ADVICE must be on or off, got %q", c.nearMissAdvice)
	}
	switch c.followAdvice {
	case "on", "true", "1", "off", "false", "0":
	default:
		bad("FOLLOW_ADVICE must be on or off, got %q", c.followAdvice)
	}
	if c.FollowAdvice && !c.NearMissAdvice {
		bad("FOLLOW_ADVICE=on needs NEAR_MISS_ADVICE=on")
	}
	if c.BOMVersions != "on" && c.BOMVersions != "off" {
		bad("BOM_VERSIONS must be on or off, got %q", c.BOMVersions)
	}
	if c.UsageMetering && !c.Teams {
		bad("USAGE_METERING=on needs TEAMS=on: usage is counted per workspace")
	}
	if c.BrokerWorkflow != "on" && c.BrokerWorkflow != "off" {
		bad("BROKER_WORKFLOW must be on or off, got %q", c.BrokerWorkflow)
	}
	if c.Teams && !strings.HasSuffix(c.TeamsAudience, ".apps.googleusercontent.com") {
		bad("TEAMS=on needs TEAMS_AUDIENCE, the Google OAuth client ID (…apps.googleusercontent.com), got %q", c.TeamsAudience)
	}
	if c.TextilePrompt != "v1" && c.TextilePrompt != "v2" {
		bad("TEXTILE_PROMPT must be v1 or v2, got %q", c.TextilePrompt)
	}
	if c.Calibration != "on" && c.Calibration != "off" {
		bad("CALIBRATION must be on or off, got %q", c.Calibration)
	}
	if c.FastPathByFamily != "on" && c.FastPathByFamily != "off" {
		bad("FAST_PATH_BY_FAMILY must be on or off, got %q", c.FastPathByFamily)
	}
	if c.PartsHTSSuggest && !strings.HasSuffix(c.PartHTSIndex, ".json") {
		bad("PART_HTS_INDEX must be a .json file written by \"corpus part-hts-index\", got %q", c.PartHTSIndex)
	}
	if c.HTSSuggest && !strings.HasSuffix(c.HTSIndex, ".json") {
		bad("HTS_INDEX must be a .json file written by \"corpus hts-index\", got %q", c.HTSIndex)
	}
	if c.StreamPredict != "on" && c.StreamPredict != "off" {
		bad("STREAM_PREDICT must be on or off, got %q", c.StreamPredict)
	}
	if c.EvalSliceMaxErrors < 0 {
		bad("EVAL_SLICE_MAX_ERRORS must be 0 or more cases, got %d", c.EvalSliceMaxErrors)
	}
	if c.EvalSliceMaxDrop < 0 {
		bad("EVAL_SLICE_MAX_DROP must be 0 or more correct cases, got %d", c.EvalSliceMaxDrop)
	}
	if c.EvalSliceBandMin <= 0 || c.EvalSliceBandMin > 1 {
		bad("EVAL_SLICE_BAND_MIN must be in (0, 1], got %v", c.EvalSliceBandMin)
	}
	if c.TextileFastPath < 0 || c.TextileFastPath > 1 {
		bad("TEXTILE_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.TextileFastPath)
	}
	if c.USMCAFastPath < 0 || c.USMCAFastPath > 1 {
		bad("USMCA_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.USMCAFastPath)
	}
	if c.PharmaFastPath < 0 || c.PharmaFastPath > 1 {
		bad("PHARMA_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.PharmaFastPath)
	}
	for name, v := range map[string]int{"RATE_BASIC_PER_MIN": c.RateBasicPerMin, "RATE_MAX_IN_FLIGHT": c.RateMaxInFlight, "RATE_AUTH_FAILURES_PER_MIN": c.RateAuthFailuresPerMin, "RATE_REVIEW_PER_MIN": c.RateReviewPerMin, "RATE_DECLARATION_PER_MIN": c.RateDeclarationPerMin, "RATE_LIGHT_PER_MIN": c.RateLightPerMin, "RATE_HEAVY_PER_MIN": c.RateHeavyPerMin, "RATE_GLOBAL_PER_MIN": c.RateGlobalPerMin, "RATE_LOOKUP_PER_MIN": c.RateLookupPerMin} {
		if v < 0 {
			bad("%s must be 0 (off) or positive, got %d", name, v)
		}
	}
	if c.RateGlobalPerMin > 0 && c.RateGlobalPerMin < 20 { // one batch request takes 20 (httpapi.MaxBatch)
		bad("RATE_GLOBAL_PER_MIN must be 0 (off) or at least 20, got %d", c.RateGlobalPerMin)
	}
	switch c.RateLimitStore {
	case RateFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when RATE_LIMIT_STORE=firestore")
		}
	case RateLocal, RateMemory:
	default:
		bad("RATE_LIMIT_STORE must be local, firestore or memory, got %q", c.RateLimitStore)
	}
	if c.AdminToken != "" && len(c.AdminToken) < 16 && c.OnCloudRun {
		bad("ADMIN_TOKEN must be at least 16 characters on Cloud Run")
	}
	if c.APIKeys {
		if c.AdminToken == "" {
			bad("API_KEYS=on needs ADMIN_TOKEN (keys are issued with it)")
		}
		if c.OnCloudRun && c.CasesStore != CasesFirestore {
			bad("API_KEYS=on on Cloud Run needs CASES_STORE=firestore (keys in memory are lost on restart and not shared)")
		}
	}
	switch c.VerifierCacheStore {
	case VerdictsFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when VERIFIER_CACHE_STORE=firestore")
		}
	case VerdictsFile:
		if c.VerifierCacheDir == "" {
			bad("VERIFIER_CACHE_STORE=file needs VERIFIER_CACHE_DIR")
		}
	case VerdictsOff:
	default:
		bad("VERIFIER_CACHE_STORE must be firestore, file or off, got %q", c.VerifierCacheStore)
	}
	// SSO (#868). Messages never include the key.
	if c.SSO {
		if !c.Teams {
			bad("SSO=on needs TEAMS=on (single sign-on signs users in to workspaces)")
		}
		if len(c.SSOSessionKey) < 32 {
			bad("SSO=on needs SSO_SESSION_KEY of at least 32 bytes (it is a secret: use a random value from the secret manager)")
		}
		if u, err := url.Parse(c.SSOSiteURL); err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") {
			bad("SSO=on needs SSO_SITE_URL, the site's https origin (e.g. https://example.com), got %q", c.SSOSiteURL)
		}
		if c.SSOSessionTTL < 5*time.Minute || c.SSOSessionTTL > 24*time.Hour {
			bad("SSO_SESSION_TTL must be between 5m and 24h, got %s", c.SSOSessionTTL)
		}
	}
	if c.SupplierDeclarations && !c.Teams {
		bad("SUPPLIER_DECLARATIONS=on needs TEAMS=on (a request is made by a case editor in a workspace)")
	}
	if c.Webhooks && !c.Teams {
		bad("WEBHOOKS=on needs TEAMS=on (endpoints belong to a workspace, and its admin registers them)")
	}
	if c.Webhooks && len(c.WebhooksSecretKey) < 32 {
		bad("WEBHOOKS=on needs WEBHOOKS_SECRET_KEY of at least 32 bytes (it seals endpoint signing secrets: use a random value from the secret manager)")
	}
	if c.AdminConsole && !c.Teams {
		bad("ADMIN_CONSOLE=on needs TEAMS=on (it is the workspace admin's overview)")
	}
	if c.SCIM && !c.SSO {
		bad("SCIM=on needs SSO=on (a SCIM token manages only domains verified on an SSO connection)")
	}
	// Audit (#284). Messages never include the key.
	if c.AuditActorKey != "" && len(c.AuditActorKey) < audit.MinKeyBytes {
		bad("AUDIT_ACTOR_KEY must be at least %d bytes (it is a secret: use a random value from the secret manager)", audit.MinKeyBytes)
	}
	if c.AuditMode == "on" && c.AuditActorKey == "" {
		bad("AUDIT=on needs AUDIT_ACTOR_KEY (at least %d bytes); use AUDIT=auto to stay off until the secret is set", audit.MinKeyBytes)
	}
	if !regexp.MustCompile(`^[a-z0-9]{1,8}$`).MatchString(c.AuditActorKeyID) {
		bad("AUDIT_ACTOR_KEY_ID must be 1 to 8 lower-case letters or digits, got %q", c.AuditActorKeyID)
	}
	if c.AuditRetentionDays < 1 || c.AuditRetentionDays > 3650 {
		bad("AUDIT_RETENTION_DAYS must be between 1 and 3650, got %d", c.AuditRetentionDays)
	}
	switch c.AuditStore {
	case CasesFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when AUDIT_STORE=firestore")
		}
	case CasesFile:
		if c.AuditDir == "" {
			bad("AUDIT_STORE=file needs AUDIT_DIR")
		}
	case CasesMemory:
	default:
		bad("AUDIT_STORE must be firestore, file or memory, got %q", c.AuditStore)
	}
	switch c.CasesStore {
	case CasesFirestore:
		if c.GCPProject == "" {
			bad("GOOGLE_CLOUD_PROJECT is required when CASES_STORE=firestore")
		}
	case CasesFile, CasesMemory:
	default:
		bad("CASES_STORE must be firestore, file or memory, got %q", c.CasesStore)
	}
	if c.VerifierSearch != "local" && c.VerifierSearch != "cross" {
		bad("VERIFIER_SEARCH must be local or cross, got %q", c.VerifierSearch)
	}
	if c.LLMMaxRetries < 0 {
		bad("LLM_MAX_RETRIES must not be negative, got %d", c.LLMMaxRetries)
	}
	if c.DutyLiveMFNTTL <= 0 {
		bad("DUTY_LIVE_MFN_TTL must be positive, got %s", c.DutyLiveMFNTTL)
	}
	if c.DutyLiveMFNTimeout <= 0 || c.DutyLiveMFNTimeout > 30*time.Second {
		bad("DUTY_LIVE_MFN_TIMEOUT must be positive and at most 30s (it delays a plan search), got %s", c.DutyLiveMFNTimeout)
	}
	if c.LLMTimeout <= 0 {
		bad("LLM_TIMEOUT must be positive, got %s", c.LLMTimeout)
	}
	if c.LLMCallGap < 0 {
		bad("LLM_CALL_GAP must be zero or positive, got %s", c.LLMCallGap)
	}
	if c.LLMCoolDown < 0 || c.LLMMaxInFlight < 0 {
		bad("LLM_COOL_DOWN and LLM_MAX_IN_FLIGHT must be zero or positive, got %s and %d", c.LLMCoolDown, c.LLMMaxInFlight)
	}
	if c.EngineAttempts < 0 {
		bad("ENGINE_ATTEMPTS must be 0 (default) or positive, got %d", c.EngineAttempts)
	}
	if c.FallbackEngineModel != "" && c.EngineAttempts < 1 {
		bad("ENGINE_ATTEMPTS must be at least 1 when FALLBACK_ENGINE_MODEL is set, got %d", c.EngineAttempts)
	}
	if c.FallbackEngineModel != "" && c.FallbackEngineLocation == "" {
		bad("FALLBACK_ENGINE_LOCATION must not be empty when FALLBACK_ENGINE_MODEL is set")
	}
	if c.FallbackFastPath < 0 || c.FallbackFastPath > 1 {
		bad("FALLBACK_FAST_PATH must be 0 (always review) or in (0, 1], got %v", c.FallbackFastPath)
	}
	return errors.Join(errs...)
}

// FamilyBars reports whether triage uses the per-subfamily bars (D63).
func (c Config) FamilyBars() bool { return c.FastPathByFamily == "on" }

// Calibrated reports whether calibrated probabilities are on (D160).
func (c Config) Calibrated() bool { return c.Calibration == "on" }

// String renders the config for startup logs; it holds no secrets.
func (c Config) String() string {
	fallback := "off"
	if c.FallbackEngineModel != "" {
		fallback = fmt.Sprintf("%s@%s attempts=%d fast_path=%.2f", c.FallbackEngineModel, c.FallbackEngineLocation, c.EngineAttempts, c.FallbackFastPath)
	}
	return fmt.Sprintf("provider=%s engine=%s router=%s verifier=%s fallback=%s k=%d fast_path=%.2f plan_conf=%.2f max_tool_calls=%d timeout=%s call_gap=%s max_in_flight=%d cloud_run=%t verifier_search=%s facts=%s cases=%s",
		c.LLMProvider, c.EngineModel, c.RouterModel, c.VerifierModel, fallback, c.PrecedentK,
		c.FastPathConfidence, c.PlanConfidence, c.VerifierMaxToolCalls, c.LLMTimeout, c.LLMCallGap, c.LLMMaxInFlight, c.OnCloudRun, c.VerifierSearch, c.FactsDir, c.CasesStore) + fmt.Sprintf(" audit=%t rate_store=%s calibration=%s calibrated_fast_path=%.2f", c.Audit, c.RateLimitStore, c.Calibration, c.CalibratedFastPath)
}

// env parses typed values and collects parse errors.
type env struct {
	get  func(string) string
	errs []error
}

// parseCIDRs accepts a comma-separated list of IP ranges. An empty list is
// valid and means "trust no proxy header"; a bad entry is a startup error so
// a typo cannot silently widen trust (#421).
func parseCIDRs(s string) ([]*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []*net.IPNet
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty CIDR in list")
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", part, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// fallbackModel treats "off" as disabled. An empty value is not reached:
// str already substitutes the default.
func fallbackModel(v string) string {
	if strings.EqualFold(v, "off") {
		return ""
	}
	return v
}

func (e *env) str(key, def string) string {
	if v := strings.TrimSpace(e.get(key)); v != "" {
		return v
	}
	return def
}

// dir is a directory setting: unset or empty means def, "off" disables it
// (returns "").
func (e *env) dir(key, def string) string {
	if v := e.str(key, def); !strings.EqualFold(v, "off") {
		return v
	}
	return ""
}

func (e *env) integer(key string, def int) int {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not an integer", key, v))
	}
	return n
}

func (e *env) float(key string, def float64) float64 {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a number", key, v))
		return 0
	}
	// ParseFloat accepts NaN and Inf. They slip past comparison-only validation
	// (every comparison with NaN is false) and, for the spend cap, silently
	// disable the cap. Reject them here with the field name attached.
	if math.IsNaN(f) || math.IsInf(f, 0) {
		e.errs = append(e.errs, fmt.Errorf("%s=%q must be a finite number", key, v))
		return 0
	}
	return f
}

func (e *env) duration(key string, def time.Duration) time.Duration {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s=%q is not a duration (e.g. 90s)", key, v))
	}
	return d
}
