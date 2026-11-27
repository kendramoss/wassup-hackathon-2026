package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fixedResolver map[string][]netip.Addr

func (r fixedResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if ips, ok := r[host]; ok {
		return ips, nil
	}
	return nil, errors.New("no such host")
}

// received is what a test receiver saw.
type received struct {
	header http.Header
	body   []byte
}

func receiver(t *testing.T, statuses ...int) (*httptest.Server, *[]received, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var got []received
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{r.Header.Clone(), b})
		mu.Unlock()
		i := int(n.Add(1)) - 1
		code := http.StatusOK
		if i < len(statuses) {
			code = statuses[i]
		} else if len(statuses) > 0 {
			code = statuses[len(statuses)-1]
		}
		if code >= 300 && code < 400 {
			w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &mu
}

func testService(t *testing.T, c *clock) (*Service, Store) {
	t.Helper()
	st := NewMemory()
	s := New(st, Options{AllowInsecure: true, Now: c.now, Client: http.DefaultClient})
	return s, st
}

func endpoint(t *testing.T, st Store, c *clock, ws, url string, events ...string) Endpoint {
	t.Helper()
	e := Endpoint{ID: NewID("wh_"), Workspace: ws, URL: url, Events: events, Enabled: true, Secret: NewSecret(), CreatedAt: c.now(), UpdatedAt: c.now()}
	if err := st.PutEndpoint(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSignAndVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"id":"evt_1"}`)
	h := Sign(now, body, "whsec_new", "whsec_old")
	if !strings.HasPrefix(h, "t=1800000000,v1=") || strings.Count(h, "v1=") != 2 {
		t.Fatalf("header = %q", h)
	}
	for _, sec := range []string{"whsec_new", "whsec_old"} {
		if err := Verify(h, body, sec, time.Minute, now); err != nil {
			t.Errorf("secret %s: %v", sec, err)
		}
	}
	if Verify(h, body, "whsec_other", time.Minute, now) == nil {
		t.Error("another secret verifies")
	}
	if Verify(h, []byte(`{"id":"evt_2"}`), "whsec_new", time.Minute, now) == nil {
		t.Error("a changed body verifies")
	}
	if Verify(h, body, "whsec_new", time.Minute, now.Add(time.Hour)) == nil {
		t.Error("an old timestamp verifies")
	}
	for _, bad := range []string{"", "v1=abc", "t=x,v1=abc", "t=1800000000"} {
		if Verify(bad, body, "whsec_new", time.Minute, now) == nil {
			t.Errorf("%q verifies", bad)
		}
	}
	if s := NewSecret(); !strings.HasPrefix(s, SecretPrefix) || len(s) != len(SecretPrefix)+43 || s == NewSecret() {
		t.Errorf("secret = %q", s)
	}
}

func TestCheckURLRefusesPrivateTargets(t *testing.T) {
	pub := netip.MustParseAddr("93.184.216.34")
	s := New(NewMemory(), Options{Resolver: fixedResolver{
		"hooks.example.com":  {pub},
		"inward.example.com": {pub, netip.MustParseAddr("10.1.2.3")},
		"meta.example.com":   {netip.MustParseAddr("169.254.169.254")},
		"six.example.com":    {netip.MustParseAddr("::1")},
	}})
	ctx := context.Background()
	for _, ok := range []string{"https://hooks.example.com/in", "https://hooks.example.com:8443/in?x=1", "https://93.184.216.34/in", "https://[2606:4700::1111]/in"} {
		if _, err := s.CheckURL(ctx, ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "hooks.example.com/in", "http://hooks.example.com/in", "ftp://hooks.example.com", "https://user:pw@hooks.example.com/in",
		"https://hooks.example.com/in#frag", "https:///in", "https://localhost/in", "https://app.localhost/in", "https://intranet/in",
		"https://metadata.google.internal/in", "https://printer.local/in",
		"https://127.0.0.1/in", "https://10.0.0.5/in", "https://172.16.0.1/in", "https://192.168.1.1/in", "https://169.254.169.254/latest",
		"https://100.64.0.1/in", "https://0.0.0.0/in", "https://[::1]/in", "https://[fe80::1]/in", "https://[fc00::1]/in", "https://[::ffff:127.0.0.1]/in",
		"https://224.0.0.1/in", "https://inward.example.com/in", "https://meta.example.com/in", "https://six.example.com/in", "https://nohost.example.com/in",
		"https://hooks.example.com:0/in", "https://hooks.example.com:99999/in", "https://" + strings.Repeat("a", MaxURLLen) + ".example.com/",
	} {
		if _, err := s.CheckURL(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted (err %v)", bad, err)
		}
	}
	// Tests may target a local receiver over http; production never does.
	loose := New(NewMemory(), Options{AllowInsecure: true})
	if _, err := loose.CheckURL(ctx, "http://127.0.0.1:8080/in"); err != nil {
		t.Errorf("insecure mode: %v", err)
	}
}

func TestEmitDeliversASignedEnvelopeToSubscribedEndpointsOnly(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t)
	hit := endpoint(t, st, c, "ws_a", srv.URL, EventRulingWatchHit, EventBatchFinished)
	endpoint(t, st, c, "ws_a", srv.URL, EventFilingStatus)   // not subscribed to this event
	endpoint(t, st, c, "ws_b", srv.URL, EventRulingWatchHit) // another workspace
	off := endpoint(t, st, c, "ws_a", srv.URL, EventRulingWatchHit)
	off.Enabled = false
	_ = st.PutEndpoint(context.Background(), off)

	s.Emit(context.Background(), "ws_a", EventRulingWatchHit, map[string]any{"case_id": "case_1", "new_origin": "Mexico"})
	s.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 1 {
		t.Fatalf("receiver saw %d requests, want 1", len(*got))
	}
	r := (*got)[0]
	if r.header.Get("Content-Type") != "application/json" || r.header.Get(EventHeader) != EventRulingWatchHit || !strings.HasPrefix(r.header.Get("User-Agent"), "UndeLabs-Webhooks/") {
		t.Errorf("headers = %v", r.header)
	}
	if err := Verify(r.header.Get(SignatureHeader), r.body, hit.Secret, time.Minute, c.now()); err != nil {
		t.Errorf("signature: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(r.body, &env); err != nil || env.Type != EventRulingWatchHit || env.Workspace != "ws_a" || !strings.HasPrefix(env.ID, "evt_") || env.Data.(map[string]any)["case_id"] != "case_1" {
		t.Errorf("envelope = %+v (%v)", env, err)
	}
	log, err := st.ListDeliveries(context.Background(), hit.ID, 10)
	if err != nil || len(log) != 1 {
		t.Fatalf("log = %+v %v", log, err)
	}
	d := log[0]
	if d.Status != StatusDelivered || len(d.Attempts) != 1 || d.Attempts[0].StatusCode != 200 || d.Attempts[0].Error != "" || d.Event != EventRulingWatchHit || d.EventID != env.ID || r.header.Get(IDHeader) != d.ID {
		t.Errorf("delivery = %+v", d)
	}
}

func TestFailedDeliveriesRetryOnTheBackoffScheduleThenGiveUp(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t, 500, 502, 200)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBudgetAlert)
	ctx := context.Background()

	s.Emit(ctx, "ws_a", EventBudgetAlert, map[string]any{"level": "alert"})
	s.Wait()
	d := onlyDelivery(t, st, e.ID)
	if d.Status != StatusPending || len(d.Attempts) != 1 || d.Attempts[0].StatusCode != 500 || d.Attempts[0].Error != "http_status" || !d.NextAttempt.Equal(c.now().Add(30*time.Second)) {
		t.Fatalf("after attempt 1: %+v", d)
	}
	// Not due yet: a sweep does nothing.
	if n, err := s.Sweep(ctx); err != nil || n != 0 {
		t.Fatalf("early sweep = %d %v", n, err)
	}
	c.advance(31 * time.Second)
	if n, err := s.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep 2 = %d %v", n, err)
	}
	d = onlyDelivery(t, st, e.ID)
	if d.Status != StatusPending || len(d.Attempts) != 2 || !d.NextAttempt.Equal(c.now().Add(2*time.Minute)) {
		t.Fatalf("after attempt 2: %+v", d)
	}
	c.advance(3 * time.Minute)
	if n, _ := s.Sweep(ctx); n != 1 {
		t.Fatalf("sweep 3 = %d", n)
	}
	d = onlyDelivery(t, st, e.ID)
	if d.Status != StatusDelivered || len(d.Attempts) != 3 || d.Attempts[2].StatusCode != 200 {
		t.Fatalf("after attempt 3: %+v", d)
	}
	mu.Lock()
	ids := map[string]bool{}
	for _, r := range *got {
		ids[r.header.Get(IDHeader)] = true
	}
	mu.Unlock()
	if len(*got) != 3 || len(ids) != 1 {
		t.Errorf("receiver saw %d requests with %d delivery ids; retries keep one id", len(*got), len(ids))
	}
	// A delivered one is never sent again.
	c.advance(24 * time.Hour)
	if n, _ := s.Sweep(ctx); n != 0 {
		t.Errorf("sweep after delivery = %d", n)
	}
}

func TestDeliveryGivesUpAfterTheLastAttempt(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, _, _ := receiver(t, 503)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	ctx := context.Background()
	s.Emit(ctx, "ws_a", EventBatchFinished, map[string]any{"total": 1})
	s.Wait()
	for i := 0; i < 10; i++ {
		c.advance(7 * time.Hour)
		_, _ = s.Sweep(ctx)
	}
	d := onlyDelivery(t, st, e.ID)
	if d.Status != StatusFailed || len(d.Attempts) != MaxAttempts || !d.NextAttempt.IsZero() {
		t.Fatalf("final: status %s, %d attempts, next %v", d.Status, len(d.Attempts), d.NextAttempt)
	}
	if MaxAttempts != 6 {
		t.Errorf("MaxAttempts = %d", MaxAttempts)
	}
}

func TestTwoSweepsNeverSendTheSameDeliveryTwice(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t, 500, 200)
	endpoint(t, st, c, "ws_a", srv.URL, EventFilingStatus)
	ctx := context.Background()
	s.Emit(ctx, "ws_a", EventFilingStatus, map[string]any{"status": "submitted"})
	s.Wait()
	c.advance(time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Sweep(ctx) }()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 2 {
		t.Errorf("receiver saw %d requests, want 2 (the failure and one retry)", len(*got))
	}
}

func TestRedirectsAreFailuresNotFollowed(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	st := NewMemory()
	s := New(st, Options{AllowInsecure: true, Now: c.now, Client: looseClient()})
	srv, got, mu := receiver(t, 302)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	s.Emit(context.Background(), "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	d := onlyDelivery(t, st, e.ID)
	mu.Lock()
	n := len(*got)
	mu.Unlock()
	if n != 1 || d.Status != StatusPending || d.Attempts[0].StatusCode != 302 || d.Attempts[0].Error != "redirect" {
		t.Fatalf("requests %d, delivery %+v", n, d)
	}
}

func TestDeliveryNeverDialsAPrivateAddress(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	st := NewMemory()
	s := New(st, Options{Now: c.now}) // the real client
	srv, got, mu := receiver(t)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished) // stored directly, as if its DNS changed after registration
	s.Emit(context.Background(), "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	d := onlyDelivery(t, st, e.ID)
	mu.Lock()
	n := len(*got)
	mu.Unlock()
	if n != 0 || len(d.Attempts) != 1 || d.Attempts[0].Error != "blocked_address" {
		t.Fatalf("requests %d, delivery %+v", n, d)
	}
}

func TestRotationSignsWithBothSecretsDuringTheGrace(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	old := e.Secret
	ctx := context.Background()

	e2, err := s.Rotate(ctx, e.ID)
	if err != nil || e2.Secret == old || e2.PrevSecret != old || !e2.PrevUntil.Equal(c.now().Add(RotationGrace)) {
		t.Fatalf("rotate = %+v %v", e2, err)
	}
	s.Emit(ctx, "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	mu.Lock()
	r := (*got)[0]
	mu.Unlock()
	for _, sec := range []string{old, e2.Secret} {
		if err := Verify(r.header.Get(SignatureHeader), r.body, sec, time.Minute, c.now()); err != nil {
			t.Errorf("during grace, secret %s...: %v", sec[:8], err)
		}
	}
	c.advance(RotationGrace + time.Second)
	s.Emit(ctx, "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	mu.Lock()
	r = (*got)[1]
	mu.Unlock()
	if Verify(r.header.Get(SignatureHeader), r.body, old, time.Minute, c.now()) == nil {
		t.Error("the old secret still signs after the grace")
	}
	if err := Verify(r.header.Get(SignatureHeader), r.body, e2.Secret, time.Minute, c.now()); err != nil {
		t.Error(err)
	}
	if _, err := s.Rotate(ctx, "wh_missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rotate missing = %v", err)
	}
}

func TestPingIsOneAttemptAndIsLogged(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t, 410)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	d, err := s.Ping(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Test || d.Event != EventTest || d.Status != StatusFailed || len(d.Attempts) != 1 || d.Attempts[0].StatusCode != 410 || !d.NextAttempt.IsZero() {
		t.Errorf("ping = %+v", d)
	}
	mu.Lock()
	r := (*got)[0]
	mu.Unlock()
	var env Envelope
	_ = json.Unmarshal(r.body, &env)
	if env.Type != EventTest || Verify(r.header.Get(SignatureHeader), r.body, e.Secret, time.Minute, c.now()) != nil {
		t.Errorf("ping body %s", r.body)
	}
	if n, _ := s.Sweep(context.Background()); n != 0 {
		t.Errorf("a failed ping is retried by a sweep (%d)", n)
	}
	if onlyDelivery(t, st, e.ID).ID != d.ID {
		t.Error("the ping is not in the delivery log")
	}
}

func TestEndpointDeletedOrDisabledStopsPendingDeliveries(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t, 500)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	ctx := context.Background()
	s.Emit(ctx, "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	e.Enabled = false
	_ = st.PutEndpoint(ctx, e)
	c.advance(time.Minute)
	_, _ = s.Sweep(ctx)
	d := onlyDelivery(t, st, e.ID)
	mu.Lock()
	n := len(*got)
	mu.Unlock()
	if n != 1 || d.Status != StatusFailed || d.Attempts[len(d.Attempts)-1].Error != "endpoint_disabled" {
		t.Fatalf("requests %d, delivery %+v", n, d)
	}
}

func onlyDelivery(t *testing.T, st Store, endpointID string) Delivery {
	t.Helper()
	ds, err := st.ListDeliveries(context.Background(), endpointID, 10)
	if err != nil || len(ds) != 1 {
		t.Fatalf("deliveries = %+v %v", ds, err)
	}
	return ds[0]
}

// storeContract runs the same checks over every Store.
func storeContract(t *testing.T, name string, open func(t *testing.T) (Store, func() Store)) {
	t.Run(name, func(t *testing.T) {
		st, reopen := open(t)
		ctx := context.Background()
		base := time.Unix(1_800_000_000, 0).UTC()
		mk := func(id, ws string, at time.Time) Endpoint {
			return Endpoint{ID: id, Workspace: ws, URL: "https://x.example.com/" + id, Events: []string{EventBatchFinished}, Enabled: true, Secret: "whsec_" + id, CreatedAt: at}
		}
		for i, e := range []Endpoint{mk("wh_2", "ws_a", base.Add(time.Second)), mk("wh_1", "ws_a", base), mk("wh_3", "ws_b", base)} {
			if err := st.PutEndpoint(ctx, e); err != nil {
				t.Fatalf("put %d: %v", i, err)
			}
		}
		got, err := st.ListEndpoints(ctx, "ws_a")
		if err != nil || len(got) != 2 || got[0].ID != "wh_1" || got[1].ID != "wh_2" || got[0].Secret != "whsec_wh_1" {
			t.Fatalf("list = %+v %v", got, err)
		}
		if subs, err := st.ListSubscribed(ctx, EventBatchFinished); err != nil || len(subs) != 3 || subs[0].Secret == "" {
			t.Fatalf("subscribed = %+v %v", subs, err)
		}
		if subs, err := st.ListSubscribed(ctx, EventEvalDrift); err != nil || len(subs) != 0 {
			t.Fatalf("subscribed to another event = %+v %v", subs, err)
		}
		if _, err := st.GetEndpoint(ctx, "wh_nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("get missing = %v", err)
		}
		e, _ := st.GetEndpoint(ctx, "wh_1")
		e.Enabled, e.PrevSecret, e.PrevUntil = false, "whsec_old", base.Add(time.Hour)
		if err := st.PutEndpoint(ctx, e); err != nil {
			t.Fatal(err)
		}
		if g, _ := st.GetEndpoint(ctx, "wh_1"); g.Enabled || g.PrevSecret != "whsec_old" || !g.PrevUntil.Equal(base.Add(time.Hour)) {
			t.Errorf("replace = %+v", g)
		}

		// Deliveries: newest first, capped per endpoint, pending ones kept.
		for i := 0; i < MaxDeliveriesKept+5; i++ {
			d := Delivery{ID: NewID("dl_"), EndpointID: "wh_2", Workspace: "ws_a", Event: EventBatchFinished, Status: StatusDelivered, CreatedAt: base.Add(time.Duration(i) * time.Second), Payload: "{}"}
			if err := st.CreateDelivery(ctx, d); err != nil {
				t.Fatal(err)
			}
		}
		pend := Delivery{ID: "dl_pending", EndpointID: "wh_2", Workspace: "ws_a", Event: EventBatchFinished, Status: StatusPending, CreatedAt: base.Add(-time.Hour), Payload: `{"a":1}`, NextAttempt: base}
		if err := st.CreateDelivery(ctx, pend); err != nil {
			t.Fatal(err)
		}
		all, _ := st.ListDeliveries(ctx, "wh_2", 0)
		if len(all) != MaxDeliveriesKept+1 {
			t.Errorf("kept %d deliveries, want %d finished plus the pending one", len(all), MaxDeliveriesKept)
		}
		top, _ := st.ListDeliveries(ctx, "wh_2", 3)
		if len(top) != 3 || !top[0].CreatedAt.After(top[1].CreatedAt) {
			t.Errorf("newest first: %+v", top)
		}
		if p, _ := st.ListPending(ctx); len(p) != 1 || p[0].ID != "dl_pending" || p[0].Payload != `{"a":1}` {
			t.Errorf("pending = %+v", p)
		}
		d, err := st.UpdateDelivery(ctx, "dl_pending", func(d *Delivery) error {
			d.Status, d.Attempts = StatusDelivered, append(d.Attempts, Attempt{At: base, StatusCode: 200})
			return nil
		})
		if err != nil || d.Status != StatusDelivered || len(d.Attempts) != 1 {
			t.Fatalf("update = %+v %v", d, err)
		}
		boom := errors.New("boom")
		if _, err := st.UpdateDelivery(ctx, "dl_pending", func(d *Delivery) error { d.Status = StatusFailed; return boom }); !errors.Is(err, boom) {
			t.Errorf("update error = %v", err)
		}
		if _, err := st.UpdateDelivery(ctx, "dl_none", func(*Delivery) error { return nil }); !errors.Is(err, ErrNotFound) {
			t.Errorf("update missing = %v", err)
		}
		if p, _ := st.ListPending(ctx); len(p) != 0 {
			t.Errorf("pending after update = %+v", p)
		}
		if reopen != nil {
			st = reopen()
			if g, err := st.GetEndpoint(ctx, "wh_1"); err != nil || g.Secret != "whsec_wh_1" || g.PrevSecret != "whsec_old" {
				t.Errorf("after reopen: %+v %v", g, err)
			}
			if d, _ := st.ListDeliveries(ctx, "wh_2", 0); len(d) != MaxDeliveriesKept+1 {
				t.Errorf("after reopen: %d deliveries", len(d))
			}
		}
		if err := st.DeleteEndpoint(ctx, "wh_2"); err != nil {
			t.Fatal(err)
		}
		if d, _ := st.ListDeliveries(ctx, "wh_2", 0); len(d) != 0 {
			t.Errorf("deliveries outlive their endpoint: %d", len(d))
		}
		if _, err := st.GetEndpoint(ctx, "wh_2"); !errors.Is(err, ErrNotFound) {
			t.Errorf("deleted endpoint = %v", err)
		}
		if g, _ := st.ListEndpoints(ctx, "ws_a"); len(g) != 1 {
			t.Errorf("after delete: %+v", g)
		}
	})
}

func TestStores(t *testing.T) {
	storeContract(t, "memory", func(*testing.T) (Store, func() Store) { return NewMemory(), nil })
	storeContract(t, "file", func(t *testing.T) (Store, func() Store) {
		dir := t.TempDir()
		st, err := NewFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		return st, func() Store {
			st2, err := NewFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			return st2
		}
	})
}

func TestValidation(t *testing.T) {
	if ev, err := ValidateEvents([]string{EventBatchFinished, EventBudgetAlert, EventBatchFinished}); err != nil || len(ev) != 2 || ev[0] != EventBatchFinished {
		t.Errorf("events = %v %v", ev, err)
	}
	for _, bad := range [][]string{nil, {}, {"nope"}, {EventTest}, {EventBatchFinished, "x"}} {
		if _, err := ValidateEvents(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v accepted", bad)
		}
	}
	if _, err := ValidateDescription(strings.Repeat("x", MaxDescriptionLen+1)); !errors.Is(err, ErrInvalid) {
		t.Error("long description accepted")
	}
}

// looseClient is the real client minus the address guard: it does not follow
// redirects, as in production, but can reach the local test receiver.
func looseClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// A platform event (one that belongs to no workspace) goes to every enabled
// endpoint subscribed to it, in every workspace, each envelope naming the
// endpoint's own workspace.
func TestEmitPlatformReachesEverySubscribedEndpointInEveryWorkspace(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	s, st := testService(t, c)
	srv, got, mu := receiver(t)
	endpoint(t, st, c, "ws_a", srv.URL, EventEvalDrift)
	endpoint(t, st, c, "ws_b", srv.URL, EventEvalDrift, EventBudgetAlert)
	endpoint(t, st, c, "ws_c", srv.URL, EventBudgetAlert) // not subscribed
	off := endpoint(t, st, c, "ws_d", srv.URL, EventEvalDrift)
	off.Enabled = false
	_ = st.PutEndpoint(context.Background(), off)

	n := s.EmitPlatform(context.Background(), EventEvalDrift, map[string]any{"series": "cbp", "month": "2026-12"})
	s.Wait()
	if n != 2 {
		t.Fatalf("queued for %d endpoints, want 2", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*got) != 2 {
		t.Fatalf("receiver saw %d requests, want 2", len(*got))
	}
	seen := map[string]bool{}
	for _, r := range *got {
		var env Envelope
		if err := json.Unmarshal(r.body, &env); err != nil || env.Type != EventEvalDrift || env.Data.(map[string]any)["month"] != "2026-12" {
			t.Fatalf("envelope %+v %v", env, err)
		}
		seen[env.Workspace] = true
	}
	if !seen["ws_a"] || !seen["ws_b"] || len(seen) != 2 {
		t.Fatalf("workspaces = %v", seen)
	}
	var nilSvc *Service
	if nilSvc.EmitPlatform(context.Background(), EventEvalDrift, nil) != 0 {
		t.Fatal("nil service queued something")
	}
}

func TestEvalDriftIsSubscribable(t *testing.T) {
	if !slices.Contains(Events, EventEvalDrift) || EventEvalDrift != "eval.drift" {
		t.Fatalf("Events = %v", Events)
	}
}
