package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
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
		}
		mux.HandleFunc("POST /v1/cases/predict", a.limit(light, a.predict))
		mux.HandleFunc("POST /v1/cases/batch", a.limit(bulk, a.batch))
		mux.HandleFunc("GET /v1/cases", a.listCases)
		mux.HandleFunc("POST /v1/products/import", a.importProduct)
		mux.HandleFunc("POST /v1/products/extract", a.limit(light, a.extractProduct))
		mux.HandleFunc("POST /v1/cases/{id}/scenarios", a.limit(light, a.scenarios))
		mux.HandleFunc("POST /v1/cases/{id}/compare-targets", a.limit(heavy, a.compareTargets))
		mux.HandleFunc("POST /v1/cases/{id}/ask", a.limit(light, a.ask))
		mux.HandleFunc("PUT /v1/cases/{id}/economics", a.putEconomics)
		mux.HandleFunc("PUT /v1/cases/{id}/pin", a.adminOnly(a.pinCase))
		mux.HandleFunc("POST /v1/cases/{id}/watch", a.limit(light, a.watch))
		mux.HandleFunc("POST /v1/watch/run", a.adminOnly(a.watchAll))
		mux.HandleFunc("GET /v1/cases/{id}", a.getCase)
		mux.HandleFunc("DELETE /v1/cases/{id}", a.deleteCase)
		mux.HandleFunc("POST /v1/cases/{id}/plan", a.limit(heavy, a.plan))
		mux.HandleFunc("POST /v1/cases/{id}/export", a.export)
		mux.HandleFunc("POST /v1/cases/{id}/answers", a.limit(light, a.answers))
		mux.HandleFunc("GET /v1/rulings/{id}", a.ruling)
		mux.HandleFunc("GET /v1/eval/summary", a.evalSummary)
		mux.HandleFunc("GET /v1/demos", a.listDemos)
		mux.HandleFunc("GET /v1/demos/{id}", a.getDemo)
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
	return withRequestLog(logger, mux)
}

type ctxKey struct{}

// RequestID returns the request ID set by the middleware, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// withRequestLog assigns each request an ID (honouring X-Request-ID from a
// caller), echoes it in the response and logs method, path, status and
// latency. Request bodies are never logged.
func withRequestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newID()
		}
		w.Header().Set("X-Request-ID", id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
		logger.InfoContext(r.Context(), "request", "request_id", id, "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "latency_ms", time.Since(start).Milliseconds())
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
