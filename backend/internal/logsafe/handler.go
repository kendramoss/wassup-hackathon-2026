package logsafe

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"
)

// Stats are cumulative counters, safe to read at any time.
type Stats struct {
	MaskedAttrs    int64 // attributes dropped or masked
	MaskedMessages int64 // messages replaced by the constant
	DroppedRecords int64 // records replaced by the constant fallback
}

type counters struct{ attrs, msgs, dropped atomic.Int64 }

// Handler is the sanitising slog.Handler.
type Handler struct {
	inner slog.Handler
	pol   *Policy
	st    *counters
	dead  bool // an unknown group name was opened: everything below it is dropped
}

// NewHandler wraps inner. It copies the policy. Wrapping a *Handler returns
// it unchanged (idempotent).
func NewHandler(inner slog.Handler, p *Policy) *Handler {
	if h, ok := inner.(*Handler); ok {
		return h
	}
	return &Handler{inner: inner, pol: p.clone(), st: &counters{}}
}

// Stats returns the counters.
func (h *Handler) Stats() Stats {
	return Stats{h.st.attrs.Load(), h.st.msgs.Load(), h.st.dropped.Load()}
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }

// sanitize appends the accepted form of a to out and reports how many
// attributes were dropped or masked. It never resolves an untrusted LogValuer.
func (h *Handler) sanitize(a slog.Attr, out []slog.Attr) ([]slog.Attr, int) {
	if a.Equal(slog.Attr{}) {
		return out, 0
	}
	if a.Value.Kind() == slog.KindGroup {
		sub := a.Value.Group()
		if a.Key != "" && !h.pol.groups[a.Key] {
			return out, 1 + countAll(sub)
		}
		var kept []slog.Attr
		n := 0
		for _, s := range sub {
			var k int
			kept, k = h.sanitize(s, kept)
			n += k
		}
		if a.Key == "" { // inline group
			return append(out, kept...), n
		}
		if len(kept) == 0 {
			return out, n
		}
		return append(out, slog.Attr{Key: a.Key, Value: slog.GroupValue(kept...)}), n
	}
	rule, ok := h.pol.keys[a.Key]
	if !ok {
		return out, 1 // the key itself may be sensitive: drop it, keep only a count
	}
	if v, ok := rule(a.Value); ok {
		return append(out, slog.Attr{Key: a.Key, Value: v}), 0
	}
	return append(out, slog.String(a.Key, masked)), 1
}

func countAll(attrs []slog.Attr) int {
	n := 0
	for _, a := range attrs {
		if a.Value.Kind() == slog.KindGroup {
			n += countAll(a.Value.Group())
		}
		n++
	}
	return n
}

// Handle implements slog.Handler. It fails closed: any panic while
// sanitising yields a constant record, never the original.
func (h *Handler) Handle(ctx context.Context, r slog.Record) (err error) {
	var nr slog.Record
	ok := func() (ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		msg := r.Message
		if !h.pol.messages[msg] {
			msg = msgMasked
			h.st.msgs.Add(1)
		}
		nr = slog.NewRecord(r.Time, r.Level, msg, r.PC)
		n := 0
		var kept []slog.Attr
		r.Attrs(func(a slog.Attr) bool {
			if h.dead {
				n += 1 + countAllOne(a)
				return true
			}
			var k int
			kept, k = h.sanitize(a, kept)
			n += k
			return true
		})
		nr.AddAttrs(kept...)
		if n > 0 {
			h.st.attrs.Add(int64(n))
			nr.AddAttrs(slog.Int(keyMaskedAttrs, n))
		}
		return true
	}()
	if ok {
		return h.forward(ctx, nr)
	}
	h.st.dropped.Add(1)
	fb := slog.NewRecord(time.Now(), r.Level, msgDropped, 0)
	fb.AddAttrs(slog.String("reason", reasonSanitizer))
	func() {
		defer func() { _ = recover() }()
		err = h.inner.Handle(ctx, fb)
	}()
	return err
}

var errInner = errors.New("logsafe: downstream handler failed")

// forward hands the sanitised record to the inner handler exactly once. A
// panic or error from it is never retried and its text is not propagated.
func (h *Handler) forward(ctx context.Context, r slog.Record) (err error) {
	defer func() {
		if recover() != nil {
			h.st.dropped.Add(1)
			err = errInner
		}
	}()
	if e := h.inner.Handle(ctx, r); e != nil {
		return errInner
	}
	return nil
}

func countAllOne(a slog.Attr) int {
	if a.Value.Kind() == slog.KindGroup {
		return countAll(a.Value.Group())
	}
	return 0
}

// WithAttrs implements slog.Handler; pre-bound attributes are sanitised too.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	if h.dead {
		c.st.attrs.Add(int64(len(attrs)))
		return &c
	}
	var kept []slog.Attr
	n := 0
	for _, a := range attrs {
		var k int
		func() {
			defer func() {
				if recover() != nil {
					k = 1
				}
			}()
			kept, k = h.sanitize(a, kept)
		}()
		n += k
	}
	h.st.attrs.Add(int64(n))
	c.inner = h.inner.WithAttrs(kept)
	return &c
}

// WithGroup implements slog.Handler. An unknown group name poisons the
// returned handler: nothing logged through it keeps any attribute.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	if h.dead || !h.pol.groups[name] {
		c.dead = true
		h.st.attrs.Add(1)
		return &c
	}
	c.inner = h.inner.WithGroup(name)
	return &c
}
