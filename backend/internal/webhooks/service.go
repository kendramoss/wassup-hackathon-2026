package webhooks

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// DefaultBackoff is the wait after each failed attempt: the first try is at
// once, then these. A delivery is tried MaxAttempts times in all.
var DefaultBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, 6 * time.Hour}

// MaxAttempts is how many times one delivery is tried with DefaultBackoff.
const MaxAttempts = 6

const (
	// lease is how long an instance that has taken a delivery to send owns
	// it: another sweep skips it, and a crash only delays the retry.
	lease      = 5 * time.Minute
	userAgent  = "UndeLabs-Webhooks/1"
	maxInFlite = 8
)

// Options configure a Service. The zero value is the production setting:
// https only, public addresses only, no redirects.
type Options struct {
	// Client sends the requests; the default is SafeClient.
	Client *http.Client
	// AllowInsecure accepts http and private targets in CheckURL (tests).
	AllowInsecure bool
	// Resolver looks up hosts in CheckURL; the default is the system's.
	Resolver Resolver
	// Backoff overrides DefaultBackoff.
	Backoff []time.Duration
	// Timeout bounds one request; the default is 10 s.
	Timeout time.Duration
	Now     func() time.Time
	Logger  *slog.Logger
}

// Service registers nothing itself: it turns events into deliveries, sends
// them and retries them. It is safe for concurrent use.
type Service struct {
	store  Store
	opts   Options
	client *http.Client
	wg     sync.WaitGroup
	sem    chan struct{}
}

// New returns a Service over store.
func New(store Store, opts Options) *Service {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.Backoff == nil {
		opts.Backoff = DefaultBackoff
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	c := opts.Client
	if c == nil {
		c = SafeClient(opts.Timeout)
	}
	return &Service{store: store, opts: opts, client: c, sem: make(chan struct{}, maxInFlite)}
}

// Store is the service's store.
func (s *Service) Store() Store { return s.store }

// Wait blocks until the deliveries Emit started have had their first attempt.
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) now() time.Time { return s.opts.Now().UTC() }

func (s *Service) maxAttempts() int { return len(s.opts.Backoff) + 1 }

// Emit queues event typ for every enabled endpoint of the workspace that
// subscribed to it and starts the first attempt in the background. It never
// fails the caller: a problem is logged and the event is lost for that
// endpoint, as for any request that did not reach it. data must not carry
// case text, comments or secrets: ids and statuses only.
func (s *Service) Emit(ctx context.Context, workspace, typ string, data any) {
	if s == nil || workspace == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	eps, err := s.store.ListEndpoints(ctx, workspace)
	if err != nil {
		s.opts.Logger.WarnContext(ctx, "webhooks: could not list endpoints", "err", logsafe.Err(err))
		return
	}
	eps = slices.DeleteFunc(eps, func(e Endpoint) bool { return !e.Subscribed(typ) })
	if len(eps) == 0 {
		return
	}
	now := s.now()
	env := Envelope{ID: NewID("evt_"), Type: typ, CreatedAt: now, Workspace: workspace, Data: data}
	body, err := json.Marshal(env)
	if err != nil || len(body) > MaxPayloadBytes {
		s.opts.Logger.WarnContext(ctx, "webhooks: event not sent: payload unusable")
		return
	}
	for _, e := range eps {
		d := Delivery{ID: NewID("dl_"), EndpointID: e.ID, Workspace: workspace, EventID: env.ID, Event: typ, Status: StatusPending,
			Attempts: []Attempt{}, NextAttempt: now.Add(lease), CreatedAt: now, Payload: string(body)}
		if err := s.store.CreateDelivery(ctx, d); err != nil {
			s.opts.Logger.WarnContext(ctx, "webhooks: could not queue a delivery", "err", logsafe.Err(err))
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.sem <- struct{}{}
			defer func() { <-s.sem }()
			s.attempt(d)
		}()
	}
}

// EmitPlatform queues event typ, which belongs to no workspace, for every
// enabled endpoint of every workspace that subscribed to it, each envelope
// naming the endpoint's own workspace, and returns how many deliveries it
// queued. It never fails the caller (see Emit). data carries ids, figures and
// statuses only.
func (s *Service) EmitPlatform(ctx context.Context, typ string, data any) int {
	if s == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	eps, err := s.store.ListSubscribed(ctx, typ)
	if err != nil {
		s.opts.Logger.WarnContext(ctx, "webhooks: could not list endpoints", "err", logsafe.Err(err))
		return 0
	}
	n := 0
	now := s.now()
	for _, e := range eps {
		env := Envelope{ID: NewID("evt_"), Type: typ, CreatedAt: now, Workspace: e.Workspace, Data: data}
		body, err := json.Marshal(env)
		if err != nil || len(body) > MaxPayloadBytes {
			s.opts.Logger.WarnContext(ctx, "webhooks: event not sent: payload unusable")
			return n
		}
		d := Delivery{ID: NewID("dl_"), EndpointID: e.ID, Workspace: e.Workspace, EventID: env.ID, Event: typ, Status: StatusPending,
			Attempts: []Attempt{}, NextAttempt: now.Add(lease), CreatedAt: now, Payload: string(body)}
		if err := s.store.CreateDelivery(ctx, d); err != nil {
			s.opts.Logger.WarnContext(ctx, "webhooks: could not queue a delivery", "err", logsafe.Err(err))
			continue
		}
		n++
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.sem <- struct{}{}
			defer func() { <-s.sem }()
			s.attempt(d)
		}()
	}
	return n
}

// Sweep sends the pending deliveries that are due and returns how many it
// tried. A scheduler calls it (POST /v1/webhooks/sweep) and the server runs
// it on a timer; several instances may sweep at once, each delivery being
// taken by one of them.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	pend, err := s.store.ListPending(ctx)
	if err != nil {
		return 0, err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	n := 0
	for _, p := range pend {
		if p.NextAttempt.After(s.now()) {
			continue
		}
		wg.Add(1)
		s.sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-s.sem }()
			var taken Delivery
			_, err := s.store.UpdateDelivery(ctx, id, func(d *Delivery) error {
				if d.Status != StatusPending || d.NextAttempt.After(s.now()) {
					return errNotDue
				}
				d.NextAttempt = s.now().Add(lease)
				taken = *d
				return nil
			})
			if err != nil {
				return
			}
			mu.Lock()
			n++
			mu.Unlock()
			s.attempt(taken)
		}(p.ID)
	}
	wg.Wait()
	return n, nil
}

var errNotDue = errors.New("webhooks: not due")

// Run sweeps every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Sweep(ctx); err != nil {
				s.opts.Logger.WarnContext(ctx, "webhooks: sweep failed", "err", logsafe.Err(err))
			}
		}
	}
}

// result is what one request came to.
type result struct {
	status int
	class  string // "" when delivered
	ms     int64
}

func (s *Service) secretsOf(e Endpoint) []string {
	if e.PrevSecret != "" && s.now().Before(e.PrevUntil) {
		return []string{e.Secret, e.PrevSecret}
	}
	return []string{e.Secret}
}

// send makes one request of d to e.
func (s *Service) send(ctx context.Context, e Endpoint, d Delivery) result {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()
	body := []byte(d.Payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL, bytes.NewReader(body))
	if err != nil {
		return result{class: "connection"}
	}
	start := time.Now()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set(IDHeader, d.ID)
	req.Header.Set(EventHeader, d.Event)
	req.Header.Set(SignatureHeader, Sign(s.now(), body, s.secretsOf(e)...))
	resp, err := s.client.Do(req)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		return result{class: errorClass(err), ms: ms}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	r := result{status: resp.StatusCode, ms: ms}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		r.class = "redirect"
	default:
		r.class = "http_status"
	}
	return r
}

func errorClass(err error) string {
	var ne net.Error
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	var re tls.RecordHeaderError
	switch {
	case errors.Is(err, errBlocked):
		return "blocked_address"
	case errors.As(err, &ua), errors.As(err, &ci), errors.As(err, &he), errors.As(err, &re):
		return "tls"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	}
	return "connection"
}

// attempt makes the next try of d, which this caller owns, and stores what
// came of it. It is the only place a delivery's status changes.
func (s *Service) attempt(d Delivery) {
	ctx, cancel := context.WithTimeout(context.Background(), s.opts.Timeout+30*time.Second)
	defer cancel()
	e, err := s.store.GetEndpoint(ctx, d.EndpointID)
	var r result
	switch {
	case errors.Is(err, ErrNotFound) || err == nil && !e.Enabled:
		r.class = "endpoint_disabled"
	case err != nil:
		s.opts.Logger.WarnContext(ctx, "webhooks: could not read an endpoint", "err", logsafe.Err(err))
		return // the lease expires and a sweep tries again
	default:
		r = s.send(ctx, e, d)
	}
	at := s.now()
	_, err = s.store.UpdateDelivery(ctx, d.ID, func(x *Delivery) error {
		x.Attempts = append(x.Attempts, Attempt{At: at, StatusCode: r.status, DurationMS: r.ms, Error: r.class})
		switch {
		case r.class == "":
			x.Status, x.NextAttempt = StatusDelivered, time.Time{}
		case r.class == "endpoint_disabled" || len(x.Attempts) >= s.maxAttempts():
			x.Status, x.NextAttempt = StatusFailed, time.Time{}
		default:
			x.Status, x.NextAttempt = StatusPending, at.Add(s.opts.Backoff[len(x.Attempts)-1])
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrNotFound) {
		s.opts.Logger.WarnContext(ctx, "webhooks: could not record an attempt", "err", logsafe.Err(err))
	}
}

// Ping sends a test event to e once and records it in the delivery log. A
// failed ping is not retried. The caller learns the outcome from the
// returned delivery; an error is a store failure.
func (s *Service) Ping(ctx context.Context, e Endpoint) (Delivery, error) {
	now := s.now()
	env := Envelope{ID: NewID("evt_"), Type: EventTest, CreatedAt: now, Workspace: e.Workspace, Data: map[string]any{"message": "This is a test event from Unde Labs.", "endpoint_id": e.ID}}
	body, err := json.Marshal(env)
	if err != nil {
		return Delivery{}, err
	}
	d := Delivery{ID: NewID("dl_"), EndpointID: e.ID, Workspace: e.Workspace, EventID: env.ID, Event: EventTest, Test: true, CreatedAt: now, Payload: string(body)}
	r := s.send(ctx, e, d)
	d.Attempts = []Attempt{{At: now, StatusCode: r.status, DurationMS: r.ms, Error: r.class}}
	d.Status = StatusDelivered
	if r.class != "" {
		d.Status = StatusFailed
	}
	if err := s.store.CreateDelivery(ctx, d); err != nil {
		return Delivery{}, err
	}
	return d, nil
}

// Rotate gives the endpoint a new secret. The old one keeps signing beside
// it for RotationGrace, so a receiver can switch without a gap.
func (s *Service) Rotate(ctx context.Context, id string) (Endpoint, error) {
	e, err := s.store.GetEndpoint(ctx, id)
	if err != nil {
		return Endpoint{}, err
	}
	now := s.now()
	e.PrevSecret, e.PrevUntil = e.Secret, now.Add(RotationGrace)
	e.Secret, e.RotatedAt, e.UpdatedAt = NewSecret(), now, now
	if err := s.store.PutEndpoint(ctx, e); err != nil {
		return Endpoint{}, err
	}
	return e, nil
}
