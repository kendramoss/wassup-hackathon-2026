package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

// Limits caps the endpoints that call a model, so a public demo cannot run
// up the model bill or tie up instances (D45). Each client gets a bucket per
// class; a global bucket caps everyone together. Zero disables a limit.
type Limits struct {
	// BasicPerMin caps the cheap routes per client: reads, renames,
	// deletes, economics, export, demos, workspaces (D137). They make no
	// model call, so they are counted on each instance only (no store write
	// per read) and not in the global bucket.
	BasicPerMin  int
	LightPerMin  int // predict, answers, ask, extract, scenarios, watch
	HeavyPerMin  int // plan search, compare targets (each runs many model calls)
	GlobalPerMin int // all clients together, in light units (heavy counts HeavyWeight)
	// LookupPerMin caps ruling lookups per client (each may fetch from CBP's
	// CROSS); they don't count against the model budget (D58).
	LookupPerMin int
	// MaxInFlight caps how many model-calling requests (light, heavy, bulk)
	// one client may have running at once on an instance (D137).
	MaxInFlight int
	// AuthFailuresPerMin caps invalid API keys per client (its address) a
	// minute, on this instance; past it the client gets 429 before any key
	// lookup (D137).
	AuthFailuresPerMin int
}

// HeavyWeight is what one heavy request takes from the global bucket.
const HeavyWeight = 5

// DefaultLimits suit a judged demo: a person clicking through never hits them.
var DefaultLimits = Limits{BasicPerMin: 120, LightPerMin: 20, HeavyPerMin: 4, GlobalPerMin: 120, MaxInFlight: 4, AuthFailuresPerMin: 10}

type class int

const (
	light class = iota
	heavy
	bulk   // batch triage: the heavy bucket per client, MaxBatch from the global bucket
	lookup // ruling lookups: their own bucket per client, not the global one
	basic  // cheap routes with no model call: their own bucket per client, this instance only (D137)
)

// rateDecision is the outcome of one rate check, with what the RateLimit-*
// response headers report: the client's bucket for the class (limit 0: not
// known, no headers).
type rateDecision struct {
	ok               bool
	wait             time.Duration // until a retry can succeed, when refused
	limit, remaining int
	reset            time.Duration // until the client's bucket is full again (or its window ends)
}

type limiter struct {
	lim     Limits
	global  *rate.Limiter
	mu      sync.Mutex
	clients map[string]*clientBuckets
	now     func() time.Time
}

type clientBuckets struct {
	basic, light, heavy, lookup *rate.Limiter
	seen                        time.Time
}

func perMin(n int) (rate.Limit, int) {
	if n <= 0 {
		return rate.Inf, 0
	}
	return rate.Limit(float64(n) / 60), n
}

func newLimiter(l Limits) *limiter {
	r, b := perMin(l.GlobalPerMin)
	return &limiter{lim: l, global: rate.NewLimiter(r, b), clients: map[string]*clientBuckets{}, now: time.Now}
}

// allow takes one request of class c for key; when refused it returns how
// long until a retry can succeed.
func (l *limiter) allow(key string, c class) (bool, time.Duration) {
	d := l.decide(key, c)
	return d.ok, d.wait
}

// decide takes one request of class c for key and reports the client
// bucket's state for the RateLimit-* headers.
func (l *limiter) decide(key string, c class) rateDecision {
	now := l.now()
	l.mu.Lock()
	cb, ok := l.clients[key]
	if !ok {
		sr, sb := perMin(l.lim.BasicPerMin)
		lr, lb := perMin(l.lim.LightPerMin)
		hr, hb := perMin(l.lim.HeavyPerMin)
		kr, kb := perMin(l.lim.LookupPerMin)
		cb = &clientBuckets{basic: rate.NewLimiter(sr, sb), light: rate.NewLimiter(lr, lb), heavy: rate.NewLimiter(hr, hb), lookup: rate.NewLimiter(kr, kb), seen: now} // seen now, or the cleanup below drops it at once
		l.clients[key] = cb
		if len(l.clients) > 10000 { // forget idle clients so the map stays bounded
			for k, v := range l.clients {
				if now.Sub(v.seen) > 10*time.Minute {
					delete(l.clients, k)
				}
			}
		}
	}
	cb.seen = now
	l.mu.Unlock()

	b, weight, n := cb.light, 1, 1
	switch c {
	case heavy:
		b, weight = cb.heavy, HeavyWeight
	case bulk: // a whole heavy burst: one batch a minute per client (D57)
		b, weight, n = cb.heavy, MaxBatch, max(1, l.lim.HeavyPerMin)
	case lookup:
		b, weight = cb.lookup, 0
	case basic:
		b, weight = cb.basic, 0
	}
	rc := b.ReserveN(now, n)
	if !rc.OK() || rc.DelayFrom(now) > 0 {
		wait := rc.DelayFrom(now)
		rc.CancelAt(now)
		return bucketState(b, now, rateDecision{wait: wait})
	}
	if weight == 0 {
		return bucketState(b, now, rateDecision{ok: true})
	}
	rg := l.global.ReserveN(now, weight)
	if !rg.OK() || rg.DelayFrom(now) > 0 {
		wait := rg.DelayFrom(now)
		rg.CancelAt(now)
		rc.CancelAt(now)
		return bucketState(b, now, rateDecision{wait: wait})
	}
	return bucketState(b, now, rateDecision{ok: true})
}

// bucketState fills d's limit, remaining and reset from bucket b at now. A
// disabled bucket (infinite rate) reports nothing.
func bucketState(b *rate.Limiter, now time.Time, d rateDecision) rateDecision {
	if b.Limit() == rate.Inf || b.Burst() <= 0 {
		return d
	}
	tokens := max(0, b.TokensAt(now))
	d.limit, d.remaining = b.Burst(), int(math.Floor(tokens))
	if missing := float64(b.Burst()) - tokens; missing > 0 {
		d.reset = time.Duration(missing / float64(b.Limit()) * float64(time.Second))
	}
	return d
}

// clientKey identifies the caller. Deployed, only the frontend can invoke the
// backend (it is not public), and the frontend's proxy forwards the browser's
// address as X-Client-IP; without it every visitor would share the
// frontend's bucket. Locally it is the connection's address. A request with
// an API key gets the key's buckets instead, wherever it comes from (D69).
func clientKey(r *http.Request) string {
	if k, ok := keyFrom(r.Context()); ok {
		return "apikey:" + k.ID
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Client-IP")); ip != "" && len(ip) <= 64 {
		return ipKey(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ipKey(host)
}

// ipKey groups IPv6 addresses by /64, the block one subscriber usually
// gets, so a client cannot rotate addresses to dodge the limits (D57).
func ipKey(s string) string {
	if ip := net.ParseIP(s); ip != nil && ip.To4() == nil {
		return ip.Mask(net.CIDRMask(64, 128)).String()
	}
	return s
}

// isAdmin reports whether the request carries the admin token in
// X-Admin-Token. Not Authorization: deployed, that header holds the Google
// identity token Cloud Run requires. With no token configured nobody is admin.
func (a *api) isAdmin(r *http.Request) bool {
	tok := r.Header.Get("X-Admin-Token")
	return a.d.AdminToken != "" && tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(a.d.AdminToken)) == 1
}

// limit wraps a model-calling handler: the daily spend caps (global, then
// this client's share), the in-flight cap, then the rate limits. Admin
// requests (the demo seed) are not limited. Model calls made under the
// request are charged to the client's share of the spend cap (D137).
func (a *api) limit(c class, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.isAdmin(r) {
			h(w, r)
			return
		}
		key := clientKey(r)
		if a.d.Spend != nil {
			if over, total := a.d.Spend.Over(r.Context()); over {
				a.overBudget(w, r, total)
				return
			}
			if over, spent, share := a.d.Spend.ClientOver(r.Context(), key); over {
				a.overClientBudget(w, r, spent, share)
				return
			}
			r = r.WithContext(spend.WithClient(r.Context(), key))
		}
		if a.inflight != nil {
			if !a.inflight.acquire(key) {
				a.tooManyInFlight(w, r)
				return
			}
			// Released when the handler returns, which for a stream is when
			// the stream ends.
			defer a.inflight.release(key)
		}
		if a.limiter != nil {
			d := a.rateCheck(r, key, c)
			setRateHeaders(w, d)
			if !d.ok {
				a.tooMany(w, d.wait)
				return
			}
		}
		h(w, r)
	}
}

// rateOnly rate-limits a handler that makes no model call (the plan
// estimate, D87; the audit trail): the spend caps and the in-flight cap do
// not apply, since it spends nothing.
func (a *api) rateOnly(c class, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.limiter != nil && !a.isAdmin(r) {
			d := a.rateCheck(r, clientKey(r), c)
			setRateHeaders(w, d)
			if !d.ok {
				a.tooMany(w, d.wait)
				return
			}
		}
		h(w, r)
	}
}

// basicDeadline bounds a cheap route's handler: it makes no model call, so
// past this the store or an outside lookup is stuck (D137).
const basicDeadline = 30 * time.Second

// cheap wraps a route that makes no model call: the basic rate class and a
// handler deadline (D137). Streaming and model-calling routes use limit,
// whose handlers run under the model's own timeouts instead.
func (a *api) cheap(h http.HandlerFunc) http.HandlerFunc {
	return a.rateOnly(basic, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), basicDeadline)
		defer cancel()
		h(w, r.WithContext(ctx))
	})
}

// setRateHeaders reports the client's bucket for the route's class in the
// IETF draft RateLimit-Limit, RateLimit-Remaining and RateLimit-Reset
// headers (seconds), when it is known.
func setRateHeaders(w http.ResponseWriter, d rateDecision) {
	if d.limit <= 0 {
		return
	}
	h := w.Header()
	h.Set("RateLimit-Limit", strconv.Itoa(d.limit))
	h.Set("RateLimit-Remaining", strconv.Itoa(max(0, d.remaining)))
	h.Set("RateLimit-Reset", strconv.Itoa(int(math.Ceil(d.reset.Seconds()))))
}

// adminOnly refuses requests without the admin token.
func (a *api) adminOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "this action needs the admin token", nil)
			return
		}
		h(w, r)
	}
}

// overBudget writes 503 "budget_reached" with Retry-After set to the next
// UTC day: the demo's daily model budget is spent (D59).
func (a *api) overBudget(w http.ResponseWriter, r *http.Request, total float64) {
	secs := int(math.Ceil(a.d.Spend.UntilReset().Seconds()))
	a.d.Logger.WarnContext(r.Context(), "daily model budget reached", "request_id", requestRef(r.Context()), "spent_usd", total, "cap_usd", a.d.Spend.Cap())
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusServiceUnavailable, "budget_reached",
		"today's demo budget for model calls is used up; the demo cases still work, and new analyses open again tomorrow (UTC)",
		map[string]int{"retry_after_s": secs})
}

// overClientBudget writes 429 "client_budget_reached" with Retry-After set
// to the next UTC day: this client has spent its share of the day's model
// budget (D137). Others can still run analyses.
func (a *api) overClientBudget(w http.ResponseWriter, r *http.Request, spent, share float64) {
	secs := int(math.Ceil(a.d.Spend.UntilReset().Seconds()))
	a.d.Logger.WarnContext(r.Context(), "client daily model budget reached", "request_id", requestRef(r.Context()), "spent_usd", spent, "cap_usd", share)
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusTooManyRequests, "client_budget_reached",
		"you have used your share of today's model budget; saved cases still work, and new analyses open again tomorrow (UTC)",
		map[string]int{"retry_after_s": secs})
}

// inFlightRetry is the Retry-After sent when a client has too many
// model-calling requests running.
const inFlightRetry = 5

// tooManyInFlight writes 429 "too_many_in_flight": the client already has
// Limits.MaxInFlight model-calling requests running on this instance.
func (a *api) tooManyInFlight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Retry-After", strconv.Itoa(inFlightRetry))
	writeError(w, http.StatusTooManyRequests, "too_many_in_flight",
		fmt.Sprintf("you have %d analyses running; wait for one to finish", a.inflight.max),
		map[string]int{"retry_after_s": inFlightRetry, "max_in_flight": a.inflight.max})
}

// inflight counts each client's running model-calling requests (D137).
type inflight struct {
	max int
	mu  sync.Mutex
	n   map[string]int // only clients with requests running; the map stays small
}

func newInflight(max int) *inflight { return &inflight{max: max, n: map[string]int{}} }

// acquire takes a slot for key, or reports false when it has max running.
func (f *inflight) acquire(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n[key] >= f.max {
		return false
	}
	f.n[key]++
	return true
}

func (f *inflight) release(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n[key] <= 1 {
		delete(f.n, key)
		return
	}
	f.n[key]--
}

// tooMany writes 429 with Retry-After.
func (a *api) tooMany(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusTooManyRequests, "rate_limited",
		fmt.Sprintf("too many requests; try again in %d s", secs), map[string]int{"retry_after_s": secs})
}
