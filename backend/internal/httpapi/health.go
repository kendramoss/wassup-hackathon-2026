package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// HealthCheck is one dependency probed by GET /health?deep=1 (D47).
type HealthCheck struct {
	Name string
	Run  func(context.Context) error
}

// HealthProbeID is the case the store check writes and deletes; it never
// shows in the case list.
const HealthProbeID = "healthcheck_probe"

// StoreCheck writes, reads and deletes a probe case, so missing permissions
// (e.g. roles/datastore.user on Firestore) show up before a user's save fails.
func StoreCheck(cs store.Cases) HealthCheck {
	return HealthCheck{Name: "case_store", Run: func(ctx context.Context) error {
		now := time.Now().UTC()
		if err := cs.Put(ctx, store.Case{ID: HealthProbeID, Date: now.Format(time.DateOnly), Facts: "health check", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if _, err := cs.Get(ctx, HealthProbeID); err != nil {
			return err
		}
		return cs.Delete(ctx, HealthProbeID)
	}}
}

// ModelCheck makes one small call to the given model.
func ModelCheck(name string, m llm.Model, modelID string) HealthCheck {
	return HealthCheck{Name: name, Run: func(ctx context.Context) error {
		_, err := m.Generate(ctx, llm.Request{Model: modelID, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with the word OK."}}, MaxOutputTokens: 256})
		return err
	}}
}

type checkResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// checkMessage is safe to return: provider bodies and store errors carry no
// credentials, but transport errors can carry a proxy URL, so those are
// reduced to their kind.
func checkMessage(err error) string {
	var apiErr *llm.APIError
	switch {
	case errors.As(err, &apiErr):
		body := apiErr.Body
		if len(body) > 300 {
			body = body[:300] + "…"
		}
		return fmt.Sprintf("model provider returned HTTP %d: %s", apiErr.StatusCode, body)
	case errors.Is(err, llm.ErrUnavailable):
		return "model provider unavailable"
	case errors.Is(err, llm.ErrBadResponse):
		return "model returned an unusable response"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out after 20 s"
	}
	var se storeErr
	if errors.As(err, &se) {
		return se.msg
	}
	return "check failed"
}

type storeErr struct{ msg string }

func (e storeErr) Error() string { return e.msg }

// health answers GET /health. With deep=1 it also runs the dependency
// checks (rate-limited like a light request, since one calls the model) and
// returns 503 when any fails.
func (a *api) health(info Info) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"status": "ok", "llm_provider": info.Provider, "version": info.Version}
		if r.URL.Query().Get("deep") != "1" || len(a.d.Checks) == 0 {
			writeJSON(w, http.StatusOK, body)
			return
		}
		a.limit(light, func(w http.ResponseWriter, r *http.Request) {
			results := map[string]checkResult{}
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, c := range a.d.Checks {
				wg.Add(1)
				go func(c HealthCheck) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
					defer cancel()
					start := time.Now()
					err := c.Run(ctx)
					res := checkResult{OK: err == nil, LatencyMS: time.Since(start).Milliseconds()}
					if err != nil {
						if c.Name == "case_store" && !errors.Is(err, context.DeadlineExceeded) {
							err = storeErr{msg: truncate(err.Error(), 300)}
						}
						res.Error = checkMessage(err)
						a.d.Logger.ErrorContext(r.Context(), "health check failed", "request_id", RequestID(r.Context()), "check", c.Name, "err", err)
					}
					mu.Lock()
					results[c.Name] = res
					mu.Unlock()
				}(c)
			}
			wg.Wait()
			status := http.StatusOK
			for _, res := range results {
				if !res.OK {
					body["status"], status = "degraded", http.StatusServiceUnavailable
				}
			}
			body["checks"] = results
			writeJSON(w, status, body)
		})(w, r)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
