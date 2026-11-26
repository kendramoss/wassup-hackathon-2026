package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// Info is static metadata returned by /health.
type Info struct {
	Provider string `json:"llm_provider"`
	Version  string `json:"version,omitempty"`
}

// NewHandler returns the API's root handler. With deps.Engine nil only
// /health is served (useful for smoke tests).
func NewHandler(logger *slog.Logger, info Info, deps ...Deps) http.Handler {
	mux := http.NewServeMux()
	var a *api
	var root http.Handler // the finished handler, for the MCP tools (set below)
	if len(deps) > 0 && deps[0].Engine != nil {
		d := deps[0]
		if d.Logger == nil {
			d.Logger = logger
		}
		a = &api{d: d, rulingMemo: map[string]rulingDTO{}, flights: newPlanFlights()}
		if d.Limits != (Limits{}) {
			a.limiter = newLimiter(d.Limits)
			if d.RateStore != nil {
				a.shared = newSharedLimiter(d.Limits, d.RateStore)
			}
			if d.Limits.MaxInFlight > 0 {
				a.inflight = newInflight(d.Limits.MaxInFlight)
			}
		}
		if d.Limits.AuthFailuresPerMin > 0 {
			a.reviewFails = newAuthFailures(d.Limits.AuthFailuresPerMin)
		}
		if d.APIKeys != nil {
			if d.Limits.AuthFailuresPerMin > 0 {
				a.authFails = newAuthFailures(d.Limits.AuthFailuresPerMin)
			}
			a.quotaLocal = newLocalQuota()
		}
		mux.HandleFunc("POST /v1/cases/predict", a.limit(light, a.predict))
		mux.HandleFunc("POST /v1/cases/batch", a.limit(bulk, a.batch))
		mux.HandleFunc("GET /v1/cases", a.cheap(a.listCases))
		mux.HandleFunc("POST /v1/products/import", a.limit(light, a.importProduct))
		if d.ERPImport {
			mux.HandleFunc("POST /v1/products/erp-import", a.limit(bulk, a.erpImport))
		}
		mux.HandleFunc("POST /v1/products/extract", a.limit(light, a.extractProduct))
		if d.MediaExtractor != nil {
			mux.HandleFunc("POST /v1/products/extract-media", a.limit(light, a.extractMedia))
		}
		mux.HandleFunc("POST /v1/products/money-prefill", a.limit(light, a.moneyPrefill))
		if d.HTS != nil {
			mux.HandleFunc("POST /v1/products/hts-suggest", a.limit(light, a.htsSuggest))
		}
		if d.PartsHTS != nil {
			mux.HandleFunc("POST /v1/products/parts-hts-suggest", a.limit(light, a.partsHTSSuggest))
		}
		if d.CostEstimator != nil {
			mux.HandleFunc("POST /v1/products/cost-estimate", a.limit(light, a.costEstimate))
		}
		if d.Family != nil {
			mux.HandleFunc("POST /v1/products/family-check", a.limit(light, a.familyCheck))
		}
		if d.BOMVersions {
			mux.HandleFunc("POST /v1/cases/{id}/bom-version", a.limit(light, a.bomVersion))
		}
		mux.HandleFunc("POST /v1/cases/{id}/scenarios", a.limit(light, a.scenarios))
		mux.HandleFunc("POST /v1/cases/{id}/compare-targets", a.limit(heavy, a.compareTargets))
		mux.HandleFunc("POST /v1/cases/{id}/ask", a.limit(light, a.ask))
		mux.HandleFunc("PUT /v1/cases/{id}/economics", a.cheap(a.putEconomics))
		mux.HandleFunc("PUT /v1/cases/{id}/pin", a.adminOnly(a.pinCase))
		mux.HandleFunc("POST /v1/cases/{id}/watch", a.limit(light, a.watch))
		mux.HandleFunc("POST /v1/watch/run", a.adminOnly(a.watchAll))
		mux.HandleFunc("GET /v1/cases/{id}", a.cheap(a.getCase))
		mux.HandleFunc("DELETE /v1/cases/{id}", a.cheap(a.deleteCase))
		mux.HandleFunc("PATCH /v1/cases/{id}", a.cheap(a.renameCase))
		mux.HandleFunc("POST /v1/cases/{id}/plan", a.limit(heavy, a.plan))
		if _, ok := d.Planner.(Estimator); ok {
			mux.HandleFunc("POST /v1/cases/{id}/plan/estimate", a.rateOnly(light, a.planEstimate))
		}
		mux.HandleFunc("POST /v1/cases/{id}/export", a.cheap(a.export))
		mux.HandleFunc("POST /v1/cases/{id}/answers", a.limit(light, a.answers))
		// Broker review links (D200): the owner's routes go through loadCase;
		// the broker's carry the link token in X-Review-Token, never in a path.
		mux.HandleFunc("POST /v1/cases/{id}/reviews", a.cheap(a.createReview))
		mux.HandleFunc("GET /v1/cases/{id}/reviews", a.cheap(a.listReviews))
		mux.HandleFunc("DELETE /v1/cases/{id}/reviews/{review}", a.cheap(a.revokeReview))
		mux.HandleFunc("GET /v1/review", a.rateOnly(review, a.getReview))
		mux.HandleFunc("POST /v1/review/decision", a.rateOnly(review, a.decideReview))
		if d.BrokerWorkflow { // review queue, discussion and sign-offs, eRulings filing package (brokerflow.go)
			mux.HandleFunc("PUT /v1/cases/{id}/queue", a.cheap(a.enqueue))
			mux.HandleFunc("POST /v1/cases/{id}/queue/transition", a.cheap(a.queueTransition))
			mux.HandleFunc("GET /v1/queue", a.cheap(a.listQueue))
			mux.HandleFunc("GET /v1/cases/{id}/collab", a.cheap(a.getCollab))
			mux.HandleFunc("POST /v1/cases/{id}/comments", a.cheap(a.addComment))
			mux.HandleFunc("POST /v1/cases/{id}/approvals", a.cheap(a.addApproval))
			mux.HandleFunc("PUT /v1/cases/{id}/collaborators", a.cheap(a.putCollaborator))
			mux.HandleFunc("GET /v1/review/comments", a.rateOnly(review, a.reviewComments))
			mux.HandleFunc("POST /v1/review/comments", a.rateOnly(review, a.postReviewComment))
			mux.HandleFunc("POST /v1/cases/{id}/filing", a.cheap(a.prepareFiling))
			mux.HandleFunc("GET /v1/cases/{id}/filing", a.cheap(a.getFiling))
			mux.HandleFunc("POST /v1/cases/{id}/filing/status", a.cheap(a.filingStatus))
		}
		mux.HandleFunc("GET /v1/rulings/{id}", a.cheap(a.ruling))
		if d.RulingSearch != nil {
			mux.HandleFunc("GET /v1/rulings", a.rateOnly(lookup, a.searchRulings))
		}
		if !d.MCPOff {
			mux.HandleFunc("POST /mcp", a.rateOnly(basic, a.mcp(&root, info.Version))) // docs/MCP.md; its tools call the routes above in-process
		}
		mux.HandleFunc("GET /v1/eval/summary", a.cheap(a.evalSummary))
		if d.Status != nil {
			mux.HandleFunc("GET /v1/status", a.cheap(d.Status.ServeHTTP))
		}
		mux.HandleFunc("GET /v1/duty-rates", a.cheap(a.dutyRates))
		mux.HandleFunc("POST /v1/rcep/check", a.cheap(a.rcepCheck))   // RCEP preferential origin, no model
		mux.HandleFunc("POST /v1/usmca/check", a.cheap(a.usmcaCheck)) // USMCA selected origin rule, no model
		if d.ATIGACheck {
			mux.HandleFunc("POST /v1/atiga/check", a.cheap(a.atigaCheck)) // ATIGA preferential origin, no model (D927)
		}
		if d.CanadaMarking != nil {
			mux.HandleFunc("POST /v1/rules/canada-marking", a.cheap(a.canadaMarking))
		}
		if d.EUOrigin != nil {
			mux.HandleFunc("POST /v1/rules/eu-origin", a.cheap(a.euOrigin)) // EU non-preferential origin, no model (D760)
		}
		if d.UKOrigin != nil {
			mux.HandleFunc("POST /v1/rules/uk-origin", a.cheap(a.ukOrigin)) // UK non-preferential origin, no model (D760)
		}
		mux.HandleFunc("PUT /v1/cases/{id}/origin-facts/{market}", a.cheap(a.putOriginFacts)) // facts supplied to the EU or UK check, no model (D801)
		mux.HandleFunc("GET /v1/cases/{id}/evidence-report", a.cheap(a.evidenceReport))       // separate checks on one product, no model (D803)
		mux.HandleFunc("PUT /v1/cases/{id}/family-facts", a.cheap(a.putFamilyFacts))          // answers to the family questions, no model (D810)
		mux.HandleFunc("GET /v1/demos", a.cheap(a.listDemos))
		mux.HandleFunc("GET /v1/jurisdictions", a.cheap(a.listJurisdictions))
		mux.HandleFunc("GET /v1/demos/{id}", a.cheap(a.getDemo))
		if d.Teams {
			mux.HandleFunc("POST /v1/workspaces", a.cheap(a.createWorkspace))
			mux.HandleFunc("GET /v1/workspaces/mine", a.cheap(a.myWorkspace))
			mux.HandleFunc("POST /v1/workspaces/{id}/members", a.cheap(a.addMember))
			mux.HandleFunc("POST /v1/workspaces/{id}/join", a.cheap(a.joinWorkspace))
			mux.HandleFunc("PUT /v1/workspaces/{id}/members/{email}/role", a.cheap(a.setMemberRole))
			mux.HandleFunc("DELETE /v1/workspaces/{id}/members/{email}", a.cheap(a.removeMember))
			if d.SSO != nil {
				mux.HandleFunc("POST /v1/sso/start", a.rateOnly(review, a.ssoStart))
				mux.HandleFunc("POST /v1/sso/oidc/callback", a.rateOnly(review, a.ssoOIDCCallback))
				mux.HandleFunc("POST /v1/sso/saml/acs", a.rateOnly(review, a.ssoSAMLACS))
				mux.HandleFunc("GET /v1/sso/saml/metadata", a.cheap(a.ssoMetadata))
				mux.HandleFunc("GET /v1/workspaces/{id}/sso", a.cheap(a.listSSOConnections))
				mux.HandleFunc("POST /v1/workspaces/{id}/sso", a.cheap(a.createSSOConnection))
				mux.HandleFunc("PUT /v1/workspaces/{id}/sso/{conn}", a.cheap(a.updateSSOConnection))
				mux.HandleFunc("POST /v1/workspaces/{id}/sso/{conn}/verify", a.cheap(a.verifySSOConnection))
				mux.HandleFunc("DELETE /v1/workspaces/{id}/sso/{conn}", a.cheap(a.deleteSSOConnection))
			}
			if d.Metering != nil {
				mux.HandleFunc("GET /v1/workspaces/{id}/usage", a.cheap(a.workspaceUsage))
				mux.HandleFunc("PUT /v1/workspaces/{id}/budget", a.cheap(a.putWorkspaceBudget))
				mux.HandleFunc("DELETE /v1/workspaces/{id}/budget", a.cheap(a.deleteWorkspaceBudget))
			}
		}
		if d.Audit != nil {
			mux.HandleFunc("GET /v1/audit", a.rateOnly(light, a.auditList))
			mux.HandleFunc("GET /v1/audit/verify", a.rateOnly(light, a.auditVerify))
			if d.AuditExport {
				mux.HandleFunc("GET /v1/audit/export", a.rateOnly(light, a.auditExport))
			}
		}
		if d.Teams && d.Retention != nil {
			mux.HandleFunc("GET /v1/workspaces/{id}/retention", a.cheap(a.getRetention))
			mux.HandleFunc("PUT /v1/workspaces/{id}/retention", a.cheap(a.putRetention))
			mux.HandleFunc("GET /v1/workspaces/{id}/tombstones", a.cheap(a.listTombstones))
		}
		if d.APIKeys != nil {
			mux.HandleFunc("POST /v1/admin/api-keys", a.adminOnly(a.issueAPIKey))
			mux.HandleFunc("GET /v1/admin/api-keys", a.adminOnly(a.listAPIKeys))
			mux.HandleFunc("DELETE /v1/admin/api-keys/{id}", a.adminOnly(a.disableAPIKey))
			// Self-service needs a verified sign-in as well as keys (#788).
			if d.Teams {
				mux.HandleFunc("POST /v1/api-keys", a.cheap(a.issueOwnAPIKey))
				mux.HandleFunc("GET /v1/api-keys", a.cheap(a.listOwnAPIKeys))
				mux.HandleFunc("DELETE /v1/api-keys/{id}", a.cheap(a.disableOwnAPIKey))
			}
		}
	}
	// Not /healthz: Cloud Run reserves paths ending in "z".
	if a != nil {
		mux.HandleFunc("GET /health", a.health(info))
	} else {
		mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "llm_provider": info.Provider, "version": info.Version})
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint", nil)
	})
	var h http.Handler = captureRoute(mux)
	if a != nil && a.d.Audit != nil {
		h = a.withAudit(mux, h)
	}
	if a != nil && a.d.Teams {
		h = a.withUser(h)
	}
	if a != nil && a.d.APIKeys != nil {
		h = a.withAPIKeys(h)
	}
	requireBearer := len(deps) > 0 && deps[0].RequireBearer
	root = withRequestLog(logger, withSecurity(requireBearer, h))
	return root
}

// withSecurity sets the response headers every API answer carries (D137):
// nosniff, no referrer, and no caching of /v1 answers (case data). With
// requireBearer it refuses, before any other check, a request other than
// /health without an "Authorization: Bearer" header: deployed, Cloud Run
// checks that token and the frontend proxy always sends one, so a request
// without it did not come through Cloud Run's front door. No CORS headers
// are ever sent: the API is for servers (X-API-Key) and for the frontend's
// own proxy, not for cross-origin browser calls.
func withSecurity(requireBearer bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/mcp" {
			h.Set("Cache-Control", "no-store")
		}
		// An MCP tool call re-enters this chain in-process (RequestURI is
		// empty only then; the server always sets it): its /mcp request was
		// already checked here, and the MCP guard drops Authorization.
		inProcess := r.RequestURI == ""
		if requireBearer && r.URL.Path != "/health" && !inProcess && !hasBearer(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "this API is reached through its gateway; the request has no bearer token", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hasBearer(r *http.Request) bool {
	scheme, tok, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	return ok && strings.EqualFold(scheme, "Bearer") && strings.TrimSpace(tok) != ""
}

type ctxKey struct{}   // the request's logsafe.Ref
type routeKey struct{} // *routeHolder

// requestRef is the request's ONE normalised id: the value of the
// X-Request-ID response header, of the request_id log field and of the audit
// event's request id. It is zero outside the middleware.
func requestRef(ctx context.Context) logsafe.Ref {
	ref, _ := ctx.Value(ctxKey{}).(logsafe.Ref)
	return ref
}

// RequestID returns the normalised request ID set by the middleware, or "".
// It is never the client's raw X-Request-ID.
func RequestID(ctx context.Context) string { return requestRef(ctx).String() }

// routeHolder carries the mux pattern that matched, from the mux wrapper back
// out to the request log (the mux sets r.Pattern on the request it serves,
// which the outer middleware, holding its own copy, cannot see).
type routeHolder struct{ pattern string }

// captureRoute wraps the mux itself so the matched pattern, never the raw
// path or query (case ids, member emails, tokens), is what gets logged.
func captureRoute(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, _ := r.Context().Value(routeKey{}).(*routeHolder); h != nil {
			defer func() { h.pattern = r.Pattern }() // set by the mux on this request
		}
		mux.ServeHTTP(w, r)
	})
}

// RoutePatterns lists every pattern the API can register (the audit coverage
// table is checked against the real route table by a test), plus the
// catch-all. The log policy registers exactly these as the closed set the
// route field may carry.
func RoutePatterns() []string {
	out := make([]string, 0, len(auditTable)+1)
	for p := range auditTable {
		out = append(out, p)
	}
	return append(out, "/")
}

// withRequestLog normalises the request ID ONCE (every X-Request-ID a caller
// sends is hashed, whatever its shape; with none the server mints one),
// echoes that value in the response, carries the same Ref in the context for
// the logs and the audit trail, and logs method, matched route pattern,
// status and latency. Request bodies, paths and queries are never logged.
func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := logsafe.RequestRef(r.Header.Get("X-Request-ID"), newID)
		w.Header().Set("X-Request-ID", ref.String())
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		holder := &routeHolder{}
		ctx := context.WithValue(context.WithValue(r.Context(), ctxKey{}, ref), routeKey{}, holder)
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))
		attrs := []any{"request_id", ref, "method", r.Method}
		if holder.pattern != "" {
			attrs = append(attrs, "route", holder.pattern)
		}
		logger.InfoContext(r.Context(), "request", append(attrs, "status", rec.status, "latency_ms", time.Since(start).Milliseconds())...)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush on the real writer (SSE).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ErrorBody is the JSON error format for every endpoint.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, ErrorBody{Code: code, Message: message, Details: details})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Encode before committing the status. encoding/json rejects values such as
	// +Inf after emitting no bytes, so writing the header first would turn a
	// serialization failure into an empty 200 that clients cannot parse.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal","message":"response could not be encoded"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// isMultipart reports whether the request body is a multipart form. Media
// types and subtypes are case-insensitive (RFC 9110), while parameter values
// such as the boundary keep their casing; net/http reads the boundary from
// the header itself when the form is parsed. A malformed or unsupported type
// falls through to the caller's JSON path, which returns a coherent error.
func isMultipart(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}
