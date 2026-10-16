package httpapi

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limits caps the endpoints that call a model, so a public demo cannot run
// up the model bill or tie up instances (D45). Each client gets a bucket per
// class; a global bucket caps everyone together. Zero disables a limit.
type Limits struct {
	LightPerMin  int // predict, answers, ask, extract, scenarios, watch
	HeavyPerMin  int // plan search, compare targets (each runs many model calls)
	GlobalPerMin int // all clients together, in light units (heavy counts HeavyWeight)
	// LookupPerMin caps ruling lookups per client (each may fetch from CBP's
	// CROSS); they don't count against the model budget (D58).
	LookupPerMin int
}

// HeavyWeight is what one heavy request takes from the global bucket.
const HeavyWeight = 5

// DefaultLimits suit a judged demo: a person clicking through never hits them.
var DefaultLimits = Limits{LightPerMin: 20, HeavyPerMin: 4, GlobalPerMin: 120}

type class int

const (
	light class = iota
	heavy
	bulk   // batch triage: the heavy bucket per client, MaxBatch from the global bucket
	lookup // ruling lookups: their own bucket per client, not the global one
)

type limiter struct {
	lim     Limits
	global  *rate.Limiter
	mu      sync.Mutex
	clients map[string]*clientBuckets
	now     func() time.Time
}

type clientBuckets struct {
	light, heavy, lookup *rate.Limiter
	seen                 time.Time
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
	now := l.now()
	l.mu.Lock()
	cb, ok := l.clients[key]
	if !ok {
		lr, lb := perMin(l.lim.LightPerMin)
		hr, hb := perMin(l.lim.HeavyPerMin)
		kr, kb := perMin(l.lim.LookupPerMin)
		cb = &clientBuckets{light: rate.NewLimiter(lr, lb), heavy: rate.NewLimiter(hr, hb), lookup: rate.NewLimiter(kr, kb), seen: now} // seen now, or the cleanup below drops it at once
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
	}
	rc := b.ReserveN(now, n)
	if !rc.OK() || rc.DelayFrom(now) > 0 {
		wait := rc.DelayFrom(now)
		rc.CancelAt(now)
		return false, wait
	}
	if weight == 0 {
		return true, 0
	}
	rg := l.global.ReserveN(now, weight)
	if !rg.OK() || rg.DelayFrom(now) > 0 {
		wait := rg.DelayFrom(now)
		rg.CancelAt(now)
		rc.CancelAt(now)
		return false, wait
	}
	return true, 0
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

// limit wraps a model-calling handler. Admin requests (the demo seed) are
// not limited.
func (a *api) limit(c class, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.d.Spend != nil && !a.isAdmin(r) {
			if over, total := a.d.Spend.Over(r.Context()); over {
				a.overBudget(w, r, total)
				return
			}
		}
		if a.limiter != nil && !a.isAdmin(r) {
			if ok, wait := a.limiter.allow(clientKey(r), c); !ok {
				a.tooMany(w, wait)
				return
			}
		}
		h(w, r)
	}
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
	a.d.Logger.WarnContext(r.Context(), "daily model budget reached", "request_id", RequestID(r.Context()), "spent_usd", total, "cap_usd", a.d.Spend.Cap())
	w.Header().Set("Retry-After", fmt.Sprint(secs))
	writeError(w, http.StatusServiceUnavailable, "budget_reached",
		"today's demo budget for model calls is used up; the demo cases still work, and new analyses open again tomorrow (UTC)",
		map[string]int{"retry_after_s": secs})
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
