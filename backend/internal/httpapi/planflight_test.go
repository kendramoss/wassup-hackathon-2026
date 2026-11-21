package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

// D402: duplicate plan searches join the one running.

func TestPlanFlightsRunOncePerKey(t *testing.T) {
	f := newPlanFlights()
	release := make(chan struct{})
	var runs atomic.Int32
	run := func(ctx context.Context) (map[string]any, error) {
		runs.Add(1)
		<-release
		return map[string]any{"n": runs.Load()}, nil
	}
	var wg sync.WaitGroup
	bodies := make([]map[string]any, 8)
	joined := make([]bool, 8)
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b, j, err := f.do(context.Background(), "k", "in", false, run)
			if err != nil {
				t.Error(err)
			}
			bodies[i], joined[i] = b, j
		}(i)
	}
	for f.waiting("k") < 8 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("ran %d times, want 1", runs.Load())
	}
	leaders := 0
	for i, b := range bodies {
		if b["n"] != int32(1) {
			t.Errorf("caller %d got %v", i, b)
		}
		if !joined[i] {
			leaders++
		}
	}
	if leaders != 1 {
		t.Errorf("%d callers started the search, want 1", leaders)
	}
	// Finished and without an idempotency key: the next call runs again.
	if _, j, _ := f.do(context.Background(), "k", "in", false, func(context.Context) (map[string]any, error) { return nil, nil }); j || f.started != 2 {
		t.Errorf("a finished search without a key must not be replayed (joined %v, started %d)", j, f.started)
	}
}

func TestPlanFlightsOutliveATimedOutCaller(t *testing.T) {
	f := newPlanFlights()
	f.grace = time.Minute
	release := make(chan struct{})
	var cancelled atomic.Bool
	run := func(ctx context.Context) (map[string]any, error) {
		select {
		case <-release:
			return map[string]any{"ok": true}, nil
		case <-ctx.Done():
			cancelled.Store(true)
			return nil, ctx.Err()
		}
	}
	// The first caller times out (the MCP client's 300 s), then retries.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := f.do(ctx, "k", "in", false, run); done <- err }()
	for f.waiting("k") < 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller: %v", err)
	}
	retry := make(chan map[string]any, 1)
	go func() { b, j, _ := f.do(context.Background(), "k", "in", false, run); retry <- b; _ = j }()
	for f.waiting("k") < 1 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	if b := <-retry; b["ok"] != true {
		t.Fatalf("retry got %v", b)
	}
	if f.started != 1 || cancelled.Load() {
		t.Errorf("started %d, cancelled %v: the retry must join the running search", f.started, cancelled.Load())
	}
}

func TestPlanFlightsCancelWhenNobodyComesBack(t *testing.T) {
	f := newPlanFlights()
	f.grace = 20 * time.Millisecond
	stopped := make(chan struct{})
	run := func(ctx context.Context) (map[string]any, error) {
		<-ctx.Done()
		close(stopped)
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _, _, _ = f.do(ctx, "k", "in", false, run) }()
	for f.waiting("k") < 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("an abandoned search kept running after the grace period")
	}
	if f.running() != 0 {
		t.Error("the abandoned search is still listed")
	}
}

func TestPlanFlightsIdempotencyKey(t *testing.T) {
	f := newPlanFlights()
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	f.now = func() time.Time { return now }
	var runs atomic.Int32
	run := func(context.Context) (map[string]any, error) {
		runs.Add(1)
		return map[string]any{"run": runs.Load()}, nil
	}
	b, j, err := f.do(context.Background(), "idem:a", "in1", true, run)
	if err != nil || j || b["run"] != int32(1) {
		t.Fatalf("first: %v %v %v", b, j, err)
	}
	b, j, err = f.do(context.Background(), "idem:a", "in1", true, run)
	if err != nil || !j || b["run"] != int32(1) || runs.Load() != 1 {
		t.Errorf("a finished search with the same key is replayed: %v %v %v", b, j, err)
	}
	if _, _, err = f.do(context.Background(), "idem:a", "in2", true, run); !errors.Is(err, errIdempotencyReuse) {
		t.Errorf("the same key with other inputs: %v", err)
	}
	now = now.Add(planReplayTTL + time.Second)
	if b, j, _ = f.do(context.Background(), "idem:a", "in2", true, run); j || b["run"] != int32(2) {
		t.Errorf("after the TTL the key is free: %v %v", b, j)
	}
	// A failed search is not kept.
	fail := func(context.Context) (map[string]any, error) { return nil, errors.New("boom") }
	_, _, _ = f.do(context.Background(), "idem:b", "in", true, fail)
	if _, j, err := f.do(context.Background(), "idem:b", "in", true, run); j || err != nil {
		t.Errorf("a failed search must run again: %v %v", j, err)
	}
}

func TestIdempotencyKeyHeader(t *testing.T) {
	for _, bad := range []string{strings.Repeat("x", idempotencyKeyMax+1), "has space", "tab\t", "é"} {
		if _, err := idempotencyKey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if k, err := idempotencyKey("retry-7f3a"); err != nil || k != "retry-7f3a" {
		t.Errorf("%q %v", k, err)
	}
}

// blockingPlanner holds every plan search until released and counts them.
type blockingPlanner struct {
	*fakePlanner
	calls   atomic.Int32
	release chan struct{}
}

func (b *blockingPlanner) Plan(ctx context.Context, r planner.Request) (planner.Result, error) {
	b.calls.Add(1)
	select {
	case <-b.release:
	case <-ctx.Done():
		return planner.Result{}, ctx.Err()
	}
	return b.fakePlanner.Plan(ctx, r)
}

func TestPlanRouteJoinsADuplicateSearch(t *testing.T) {
	bp := &blockingPlanner{fakePlanner: &fakePlanner{}, release: make(chan struct{})}
	h := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Planner = bp })
	_, c := h.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	post := func(body string, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.h.ServeHTTP(w, req)
		return w
	}
	joins := planJoins.Load()
	var wg sync.WaitGroup
	res := make([]*httptest.ResponseRecorder, 3)
	for i := range res {
		wg.Add(1)
		go func(i int) { defer wg.Done(); res[i] = post(`{"target":"Thailand"}`, nil) }(i)
	}
	for bp.calls.Load() < 1 || planJoins.Load()-joins < 2 {
		time.Sleep(time.Millisecond)
	}
	close(bp.release)
	wg.Wait()
	if n := bp.calls.Load(); n != 1 {
		t.Fatalf("planner ran %d times for 3 identical requests, want 1", n)
	}
	for i, w := range res {
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Make Motor-fan assembly and PCBA (SMT) in Thailand") {
			t.Errorf("request %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	// Other inputs are another search.
	if w := post(`{"target":"Vietnam"}`, nil); w.Code != http.StatusOK || bp.calls.Load() != 2 {
		t.Errorf("Vietnam: %d, calls %d", w.Code, bp.calls.Load())
	}
	// An idempotency key replays a finished search, refuses other inputs
	// and rejects a malformed key.
	key := map[string]string{"Idempotency-Key": "plan-retry-1"}
	if w := post(`{"target":"Thailand"}`, key); w.Code != http.StatusOK || bp.calls.Load() != 3 {
		t.Fatalf("keyed: %d, calls %d", w.Code, bp.calls.Load())
	}
	if w := post(`{"target":"Thailand"}`, key); w.Code != http.StatusOK || bp.calls.Load() != 3 {
		t.Errorf("keyed retry must replay: %d, calls %d", w.Code, bp.calls.Load())
	}
	if w := post(`{"target":"Vietnam"}`, key); w.Code != http.StatusConflict {
		t.Errorf("key reused with other inputs: %d %s", w.Code, w.Body.String())
	}
	if w := post(`{"target":"Thailand"}`, map[string]string{"Idempotency-Key": "has space"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad key: %d", w.Code)
	}
}

// exitPlanner is a blockingPlanner that also counts searches that returned.
type exitPlanner struct {
	*blockingPlanner
	exits atomic.Int32
}

func (e *exitPlanner) Plan(ctx context.Context, r planner.Request) (planner.Result, error) {
	defer e.exits.Add(1)
	return e.blockingPlanner.Plan(ctx, r)
}

// planPoster posts plan requests for one client to a fresh case.
func planPoster(t *testing.T, p Planner, maxInFlight int) func(ctx context.Context, target string) *httptest.ResponseRecorder {
	h := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Planner = p
		d.Limits = Limits{BasicPerMin: 1000, LightPerMin: 1000, HeavyPerMin: 1000, GlobalPerMin: 10000, MaxInFlight: maxInFlight}
	})
	_, c := h.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	return func(ctx context.Context, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, "POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"`+target+`"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.h.ServeHTTP(w, req)
		return w
	}
}

func tooManyInFlight(w *httptest.ResponseRecorder) bool {
	return w.Code == http.StatusTooManyRequests && strings.Contains(w.Body.String(), "too_many_in_flight")
}

// #581: a plan search that keeps running after its caller left keeps the
// caller's in-flight slot until it ends, so one client cannot run more
// searches at once than MaxInFlight by dropping requests.
func TestDetachedPlanSearchKeepsItsInFlightSlot(t *testing.T) {
	ep := &exitPlanner{blockingPlanner: &blockingPlanner{fakePlanner: &fakePlanner{}, release: make(chan struct{})}}
	post := planPoster(t, ep, 1)
	bg := context.Background()

	// Start a search and drop the request while the search runs.
	ctx, cancel := context.WithCancel(bg)
	done := make(chan struct{})
	go func() { defer close(done); post(ctx, "Thailand") }()
	for ep.calls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if ep.exits.Load() != 0 {
		t.Fatalf("the dropped search stopped; it should run on in its grace period")
	}
	// The same client asks for other searches: refused, no new search.
	// (Each is bounded so that a regression fails instead of hanging.)
	for _, target := range []string{"Vietnam", "Mexico", "India"} {
		tctx, tcancel := context.WithTimeout(bg, 2*time.Second)
		w := post(tctx, target)
		tcancel()
		if !tooManyInFlight(w) {
			t.Errorf("%s while the first search runs detached = %d %s, want 429 too_many_in_flight", target, w.Code, w.Body.String())
		}
	}
	if n := ep.calls.Load(); n != 1 {
		t.Errorf("planner searches = %d, want 1", n)
	}
	// Once the detached search ends its slot comes back.
	close(ep.release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := post(bg, "Vietnam")
		if w.Code == http.StatusOK {
			break
		}
		if !tooManyInFlight(w) || time.Now().After(deadline) {
			t.Fatalf("after the first search ended = %d %s", w.Code, w.Body.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := ep.calls.Load(); n != 2 {
		t.Errorf("planner searches = %d, want 2", n)
	}
	// The slots all came back: another search fits under MaxInFlight 1.
	if w := post(bg, "Mexico"); w.Code != http.StatusOK {
		t.Errorf("a later search = %d %s", w.Code, w.Body.String())
	}
}

// A request that joins a running search holds its own slot only while it
// waits; the search's slot is the starter's, counted once.
func TestJoiningPlanRequestHoldsASlotOnlyWhileWaiting(t *testing.T) {
	bp := &blockingPlanner{fakePlanner: &fakePlanner{}, release: make(chan struct{})}
	post := planPoster(t, bp, 2)
	bg := context.Background()
	joins := planJoins.Load()
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); post(bg, "Thailand") }()
	}
	for bp.calls.Load() < 1 || planJoins.Load()-joins < 1 {
		time.Sleep(time.Millisecond)
	}
	// The starter and the joiner fill the two slots.
	tctx, tcancel := context.WithTimeout(bg, 2*time.Second)
	if w := post(tctx, "Vietnam"); !tooManyInFlight(w) {
		t.Errorf("third request with a starter and a joiner waiting = %d, want 429", w.Code)
	}
	tcancel()
	close(bp.release)
	wg.Wait()
	// Both slots are back: two more searches fit side by side.
	var ok atomic.Int32
	for _, target := range []string{"Vietnam", "Mexico"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := post(bg, target); w.Code == http.StatusOK {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 2 {
		t.Errorf("searches after the joined one = %d of 2", ok.Load())
	}
}
