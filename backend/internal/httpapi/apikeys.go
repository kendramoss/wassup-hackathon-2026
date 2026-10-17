package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
)

// API keys for brokers (D69). With Deps.APIKeys set, a request may carry
// "X-API-Key: op_...". A valid key gets its own rate-limit buckets instead of
// the X-Client-IP ones, a request quota per UTC day, and owns the cases it
// creates: they are visible to that key and the admin only, whatever
// CASE_OWNERSHIP says, and the key sees only its own cases and the pinned
// demos. An unknown or disabled key is a 401, never a silent fall back to
// anonymous access. Model calls made for a key still count toward the
// daily spend cap (D59), which refuses keys like everyone else.

type apiKeyCtx struct{}

// keyFrom returns the request's authenticated API key, if any.
func keyFrom(ctx context.Context) (apikey.Key, bool) {
	k, ok := ctx.Value(apiKeyCtx{}).(apikey.Key)
	return k, ok
}

// keyOwnerPrefix marks an owner hash that belongs to an API key, so a
// broker's case stays hidden from visitors even with CASE_OWNERSHIP off.
const keyOwnerPrefix = "key_"

// keyOwner is the owner stored on cases created with key: a hash of its ID.
func keyOwner(k apikey.Key) string {
	h := sha256.Sum256([]byte("origin-proof-apikey:" + k.ID))
	return keyOwnerPrefix + hex.EncodeToString(h[:16])
}

// withAPIKeys checks X-API-Key on /v1 requests and counts it against the
// key's daily quota. Requests without the header pass through unchanged.
func (a *api) withAPIKeys(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if secret == "" || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		key, err := a.d.APIKeys.Authenticate(r.Context(), secret)
		if errors.Is(err, apikey.ErrInvalid) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "the API key is invalid or disabled", nil)
			return
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		ok, used, reset, err := a.d.APIKeys.Use(r.Context(), key)
		if err != nil { // the counter is unavailable: serve, the rate limits still apply
			a.d.Logger.WarnContext(r.Context(), "api key quota not counted", "request_id", RequestID(r.Context()), "key_id", key.ID, "err", err)
		}
		if !ok {
			secs := max(1, int(math.Ceil(reset.Seconds())))
			a.d.Logger.WarnContext(r.Context(), "api key quota exceeded", "request_id", RequestID(r.Context()), "key_id", key.ID, "used", used, "quota", key.DailyQuota)
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			writeError(w, http.StatusTooManyRequests, "quota_exceeded",
				fmt.Sprintf("this API key's daily quota of %d requests is used up; it resets at 00:00 UTC", key.DailyQuota),
				map[string]int{"retry_after_s": secs, "daily_quota": key.DailyQuota})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiKeyCtx{}, key)))
	})
}

type issueKeyRequest struct {
	Name       string `json:"name"`
	DailyQuota int    `json:"daily_quota"`
}

// issueAPIKey creates a key and returns its secret once (admin only).
func (a *api) issueAPIKey(w http.ResponseWriter, r *http.Request) {
	var in issueKeyRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	secret, k, err := a.d.APIKeys.Issue(r.Context(), in.Name, in.DailyQuota)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.d.Logger.InfoContext(r.Context(), "api key issued", "request_id", RequestID(r.Context()), "key_id", k.ID, "name", k.Name, "daily_quota", k.DailyQuota)
	writeJSON(w, http.StatusCreated, map[string]any{"key": secret, "api_key": k})
}

// listAPIKeys lists every key without secrets (admin only).
func (a *api) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	ks, err := a.d.APIKeys.List(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if ks == nil {
		ks = []apikey.Key{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": ks})
}

// disableAPIKey turns a key off for good (admin only). The record stays, so
// the list shows what was issued.
func (a *api) disableAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.d.APIKeys.Disable(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	a.d.Logger.InfoContext(r.Context(), "api key disabled", "request_id", RequestID(r.Context()), "key_id", id)
	w.WriteHeader(http.StatusNoContent)
}
