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
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
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
	// legacy name kept: the salt is part of every stored hash; changing it orphans existing records (D-rename).
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
		// Throttle guessing before the key lookup: a client (its address;
		// no key is known yet) that has failed too often waits (D137).
		// Reserve a slot under the bucket's lock, so concurrent guesses
		// cannot all observe a free bucket before any failure is recorded
		// (#386). Only an invalid key consumes a failure token; a valid
		// key gives the slot back.
		client := a.clientKey(r)
		release, ok, wait := a.authFails.begin(client)
		if !ok {
			a.d.Logger.WarnContext(r.Context(), "api key failures limited", "request_id", requestRef(r.Context()))
			secs := max(1, int(math.Ceil(wait.Seconds())))
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			writeError(w, http.StatusTooManyRequests, "auth_rate_limited",
				fmt.Sprintf("too many invalid API keys from this client; try again in %d s", secs), map[string]int{"retry_after_s": secs})
			return
		}
		key, err := a.d.APIKeys.Authenticate(r.Context(), secret)
		if errors.Is(err, apikey.ErrInvalid) {
			a.authFails.fail(client)
			release()
			a.auditAuthDenied(r)
			writeError(w, http.StatusUnauthorized, "unauthorized", "the API key is invalid or disabled", nil)
			return
		}
		if err != nil {
			release()
			a.fail(w, r, err)
			return
		}
		ok, used, reset, err := a.d.APIKeys.Use(r.Context(), key)
		if err != nil {
			// The shared counter is unavailable: serve (fail open, for
			// availability), but count on this instance so the key still
			// stops at its quota here (D137). The rate limits still apply.
			a.d.Logger.WarnContext(r.Context(), "api key quota not counted", "request_id", requestRef(r.Context()), keyAttr(key.ID), "err", logsafe.Err(err))
			ok, used = a.quotaLocal.use(key.ID, key.DailyQuota, time.Now())
		}
		if !ok {
			release()
			secs := max(1, int(math.Ceil(reset.Seconds())))
			a.d.Logger.WarnContext(r.Context(), "api key quota exceeded", "request_id", requestRef(r.Context()), keyAttr(key.ID), "used", used, "quota", key.DailyQuota)
			w.Header().Set("Retry-After", fmt.Sprint(secs))
			writeError(w, http.StatusTooManyRequests, "quota_exceeded",
				fmt.Sprintf("this API key's daily quota of %d requests is used up; it resets at 00:00 UTC", key.DailyQuota),
				map[string]int{"retry_after_s": secs, "daily_quota": key.DailyQuota})
			return
		}
		release()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiKeyCtx{}, key)))
	})
}

type issueKeyRequest struct {
	Name       string `json:"name"`
	DailyQuota int    `json:"daily_quota"`
}

// selfServiceKeyQuota is the fixed daily request quota a signed-in user gets
// (#788). Users cannot mint a larger one by sending daily_quota.
const selfServiceKeyQuota = 200

type issueOwnKeyRequest struct {
	Name string `json:"name"`
}

// issueAPIKey creates a key and returns its secret once (admin only).
func (a *api) issueAPIKey(w http.ResponseWriter, r *http.Request) {
	var in issueKeyRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := apikey.ValidateIssue(in.Name, in.DailyQuota); err != nil { // refuse a bad request before any intent
		a.fail(w, r, err)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: "platform", Object: "apikey:new"}); err != nil {
		a.fail(w, r, err)
		return
	}
	secret, k, err := a.d.APIKeys.Issue(r.Context(), in.Name, in.DailyQuota)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if op := auditOpOf(r.Context()); op != nil && op.intent != nil {
		op.object = objectRef("apikey", k.ID) // the completion names the key that was made
	}
	a.d.Logger.InfoContext(r.Context(), "api key issued", "request_id", requestRef(r.Context()), keyAttr(k.ID), "daily_quota", k.DailyQuota)
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
	a.auditSingle(r, auditStep{Tenant: "platform"})
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": ks})
}

// disableAPIKey turns a key off for good (admin only). The record stays, so
// the list shows what was issued.
func (a *api) disableAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.d.APIKeys.Exists(r.Context(), id); err != nil { // an unknown key is a 404 before any intent
		a.fail(w, r, err)
		return
	}
	if err := a.auditBegin(r, auditStep{Tenant: "platform", Object: objectRef("apikey", id)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.APIKeys.Disable(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	// id came from the path, but Exists found it in the key store above and
	// Disable acted on it, so it is a stored key id (a name that is not one
	// was a 404 before this line). keyAttr drops anything without that shape.
	a.d.Logger.InfoContext(r.Context(), "api key disabled", "request_id", requestRef(r.Context()), keyAttr(id))
	w.WriteHeader(http.StatusNoContent)
}

// issueOwnAPIKey creates a key for the verified signed-in user (#788). The
// secret is shown once; the owner comes from the checked sign-in, never from
// the request body, and the quota is fixed on the server.
func (a *api) issueOwnAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	var in issueOwnKeyRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := apikey.ValidateIssue(in.Name, selfServiceKeyQuota); err != nil { // bad request before any intent
		a.fail(w, r, err)
		return
	}
	tenant := a.requesterTenant(a.principalOf(r))
	if err := a.auditBegin(r, auditStep{Tenant: tenant, Object: "apikey:new"}); err != nil {
		a.fail(w, r, err)
		return
	}
	secret, k, err := a.d.APIKeys.IssueFor(r.Context(), id.owner, in.Name, selfServiceKeyQuota)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if op := auditOpOf(r.Context()); op != nil && op.intent != nil {
		op.object = objectRef("apikey", k.ID)
	}
	a.d.Logger.InfoContext(r.Context(), "api key issued", "request_id", requestRef(r.Context()), keyAttr(k.ID), "daily_quota", k.DailyQuota)
	writeJSON(w, http.StatusCreated, map[string]any{"key": secret, "api_key": k})
}

// listOwnAPIKeys lists only the signed-in user's keys, without secrets.
func (a *api) listOwnAPIKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	ks, err := a.d.APIKeys.ListForOwner(r.Context(), id.owner)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if ks == nil {
		ks = []apikey.Key{}
	}
	a.auditSingle(r, auditStep{Tenant: a.requesterTenant(a.principalOf(r))})
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": ks})
}

// disableOwnAPIKey revokes one of the signed-in user's keys for good. Another
// user's id, an admin key or an unknown id is a 404: ids do not leak.
func (a *api) disableOwnAPIKey(w http.ResponseWriter, r *http.Request) {
	id, ok := requireUser(w, r)
	if !ok {
		return
	}
	keyID := r.PathValue("id")
	if err := a.d.APIKeys.ExistsForOwner(r.Context(), id.owner, keyID); err != nil {
		if errors.Is(err, apikey.ErrNotFound) {
			// A stored admin key or another account's key is hidden behind the
			// same 404 as an unknown id, but unlike an unknown id it is a probe
			// worth recording in the requester's tenant.
			if existsErr := a.d.APIKeys.Exists(r.Context(), keyID); existsErr == nil {
				auditHidden(r)
			}
		}
		a.fail(w, r, err)
		return
	}
	tenant := a.requesterTenant(a.principalOf(r))
	if err := a.auditBegin(r, auditStep{Tenant: tenant, Object: objectRef("apikey", keyID)}); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.d.APIKeys.DisableForOwner(r.Context(), id.owner, keyID); err != nil {
		a.fail(w, r, err)
		return
	}
	a.d.Logger.InfoContext(r.Context(), "api key disabled", "request_id", requestRef(r.Context()), keyAttr(keyID))
	w.WriteHeader(http.StatusNoContent)
}

// authFailures counts invalid API keys per client, on this instance: a
// bucket of Limits.AuthFailuresPerMin that refills over a minute. Only
// failures take from it.
type authFailures struct {
	perMin  int
	mu      sync.Mutex
	clients map[string]*failBucket
	now     func() time.Time
}

type failBucket struct {
	b        *rate.Limiter
	seen     time.Time
	inflight int // lookups in progress, counted under authFailures.mu (#386)
}

func newAuthFailures(perMin int) *authFailures {
	return &authFailures{perMin: perMin, clients: map[string]*failBucket{}, now: time.Now}
}

func (f *authFailures) bucket(client string, now time.Time) *failBucket {
	fb, ok := f.clients[client]
	if !ok {
		fb = &failBucket{b: rate.NewLimiter(rate.Limit(float64(f.perMin)/60), f.perMin)}
		f.clients[client] = fb
		if len(f.clients) > 10000 { // forget idle clients so the map stays bounded
			for k, v := range f.clients {
				if now.Sub(v.seen) > 10*time.Minute {
					delete(f.clients, k)
				}
			}
		}
	}
	fb.seen = now
	return fb
}

// begin reserves one key-lookup slot for client and returns the function
// that gives it back. A slot is separate from the failure token: an invalid
// key also spends a token in fail, while a valid key only returns the slot.
// Checking and reserving happen under the same lock, so N concurrent guesses
// cannot all pass the check before any of them reaches the store (#386).
// A lookup in flight holds one of the bucket's tokens until it finishes: a
// lookup is admitted only while the tokens left, less the lookups in flight,
// are at least one. Guesses that arrive while earlier ones are still in the
// store therefore cannot reach it more than perMin times a minute (#505).
// When no token is free, the lookup is refused with the refill wait.
func (f *authFailures) begin(client string) (release func(), ok bool, wait time.Duration) {
	if f == nil {
		return func() {}, true, 0
	}
	f.mu.Lock()
	now := f.now()
	fb := f.bucket(client, now)
	if free := fb.b.TokensAt(now) - float64(fb.inflight); free < 1 {
		f.mu.Unlock()
		return nil, false, max(time.Second/10, time.Duration((1-free)/float64(fb.b.Limit())*float64(time.Second)))
	}
	fb.inflight++
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		fb.inflight--
		f.mu.Unlock()
	}, true, 0
}

// fail records one invalid key from client. It always takes a token, into
// debt if the bucket is already empty, so a failure recorded late still
// lengthens the client's wait (#505).
func (f *authFailures) fail(client string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	f.bucket(client, now).b.ReserveN(now, 1)
}

// localQuota counts API-key requests on this instance while the shared
// usage counter is unavailable, per key and UTC day.
type localQuota struct {
	mu   sync.Mutex
	day  string
	used map[string]int64
}

func newLocalQuota() *localQuota { return &localQuota{used: map[string]int64{}} }

// use counts one request for id now and reports whether it is within quota,
// with the count.
func (q *localQuota) use(id string, quota int, now time.Time) (bool, int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if day := now.UTC().Format(time.DateOnly); day != q.day {
		q.day, q.used = day, map[string]int64{}
	}
	q.used[id]++
	return q.used[id] <= int64(quota), q.used[id]
}
