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
	if len(deps) > 0 && deps[0].Engine != nil {
		d := deps[0]
		if d.Logger == nil {
			d.Logger = logger
		}
		a = &api{d: d, rulingMemo: map[string]rulingDTO{}}
		if d.Limits != (Limits{}) {
			a.limiter = newLimiter(d.Limits)
			if d.RateStore != nil {
				a.shared = newSharedLimiter(d.Limits, d.RateStore)
			}
		}
		mux.HandleFunc("POST /v1/cases/predict", a.limit(light, a.predict))
		mux.HandleFunc("POST /v1/cases/batch", a.limit(bulk, a.batch))
		mux.HandleFunc("GET /v1/cases", a.listCases)
		mux.HandleFunc("POST /v1/products/import", a.limit(light, a.importProduct))
		mux.HandleFunc("POST /v1/products/extract", a.limit(light, a.extractProduct))
		mux.HandleFunc("POST /v1/products/money-prefill", a.limit(light, a.moneyPrefill))
		if d.HTS != nil {
			mux.HandleFunc("POST /v1/products/hts-suggest", a.limit(light, a.htsSuggest))
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
		mux.HandleFunc("PUT /v1/cases/{id}/economics", a.putEconomics)
		mux.HandleFunc("PUT /v1/cases/{id}/pin", a.adminOnly(a.pinCase))
		mux.HandleFunc("POST /v1/cases/{id}/watch", a.limit(light, a.watch))
		mux.HandleFunc("POST /v1/watch/run", a.adminOnly(a.watchAll))
		mux.HandleFunc("GET /v1/cases/{id}", a.getCase)
		mux.HandleFunc("DELETE /v1/cases/{id}", a.deleteCase)
		mux.HandleFunc("PATCH /v1/cases/{id}", a.renameCase)
		mux.HandleFunc("POST /v1/cases/{id}/plan", a.limit(heavy, a.plan))
		if _, ok := d.Planner.(Estimator); ok {
			mux.HandleFunc("POST /v1/cases/{id}/plan/estimate", a.rateOnly(light, a.planEstimate))
		}
		mux.HandleFunc("POST /v1/cases/{id}/export", a.export)
		mux.HandleFunc("POST /v1/cases/{id}/answers", a.limit(light, a.answers))
		mux.HandleFunc("GET /v1/rulings/{id}", a.ruling)
		mux.HandleFunc("GET /v1/eval/summary", a.evalSummary)
		mux.HandleFunc("GET /v1/duty-rates", a.dutyRates)
		mux.HandleFunc("GET /v1/demos", a.listDemos)
		mux.HandleFunc("GET /v1/demos/{id}", a.getDemo)
		if d.Teams {
			mux.HandleFunc("POST /v1/workspaces", a.createWorkspace)
			mux.HandleFunc("GET /v1/workspaces/mine", a.myWorkspace)
			mux.HandleFunc("POST /v1/workspaces/{id}/members", a.addMember)
			mux.HandleFunc("POST /v1/workspaces/{id}/join", a.joinWorkspace)
			mux.HandleFunc("PUT /v1/workspaces/{id}/members/{email}/role", a.setMemberRole)
			mux.HandleFunc("DELETE /v1/workspaces/{id}/members/{email}", a.removeMember)
		}
		if d.Audit != nil {
			mux.HandleFunc("GET /v1/audit", a.rateOnly(light, a.auditList))
			mux.HandleFunc("GET /v1/audit/verify", a.rateOnly(light, a.auditVerify))
		}
		if d.APIKeys != nil {
			mux.HandleFunc("POST /v1/admin/api-keys", a.adminOnly(a.issueAPIKey))
			mux.HandleFunc("GET /v1/admin/api-keys", a.adminOnly(a.listAPIKeys))
			mux.HandleFunc("DELETE /v1/admin/api-keys/{id}", a.adminOnly(a.disableAPIKey))
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
	return withRequestLog(logger, h)
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
