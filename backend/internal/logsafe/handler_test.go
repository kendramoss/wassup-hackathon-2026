package logsafe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type tripValuer struct{ called *bool }

func (v tripValuer) LogValue() slog.Value {
	*v.called = true
	panic("valuer must not run: " + sentSupplier)
}

type tripStringer struct{ called *bool }

func (s tripStringer) String() string { *s.called = true; return sentSupplier }

type secretStruct struct{ Supplier, BOM string }

func TestUnknownKeyDroppedAndCounted(t *testing.T) {
	l, buf, h := newTest(t, DefaultPolicy())
	l.InfoContext(bg, "request", sentKeyName, sentSupplier, "status", 200, "latency_ms", int64(7))
	out := buf.String()
	assertClean(t, out)
	if !strings.Contains(out, `"masked_attrs":1`) || !strings.Contains(out, `"status":200`) {
		t.Fatalf("want masked_attrs=1 and status kept: %s", out)
	}
	if h.Stats().MaskedAttrs != 1 {
		t.Fatalf("stats: %+v", h.Stats())
	}
}

func TestGroupsWithAttrsAndWithGroupAreSanitised(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	l.InfoContext(bg, "request", slog.Group(sentGroup, slog.String("model", sentSupplier), slog.Int("status", 1)), slog.Group("", slog.Int("status", 200)))
	l.With(sentKeyName, sentBOM, "status", 201).InfoContext(bg, "request")
	l.With("model_id", sentEmail).InfoContext(bg, "request")
	l.WithGroup(sentGroup).With(slog.String("model", sentSupplier)).InfoContext(bg, "request", "status", 5, sentKeyName, sentToken)
	l.WithGroup(sentGroup).WithGroup(sentGroup).InfoContext(bg, "request", slog.Group(sentGroup, slog.String("a", sentBOM)))
	assertClean(t, buf.String())
	if !strings.Contains(buf.String(), `"status":200`) || !strings.Contains(buf.String(), `"status":201`) {
		t.Fatalf("inline group and With status lost: %s", buf.String())
	}
}

func TestLogValuerNotCalledUnlessTrusted(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	var a, b, c bool
	l.InfoContext(bg, "request", sentKeyName, tripValuer{&a}, "status", tripValuer{&b}, "model", slog.AnyValue(tripValuer{&c}))
	assertClean(t, buf.String())
	if a || b || c {
		t.Fatalf("untrusted LogValuer was called: %v %v %v", a, b, c)
	}
}

func TestAnyStructMapStringerMasked(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	var s bool
	l.InfoContext(bg, "request",
		"regime", secretStruct{sentSupplier, sentBOM},
		"method", map[string]string{"k": sentBOM},
		"provider", tripStringer{&s},
		"finish_reason", []string{sentSupplier},
		"class", []any{sentBOM},
		"status", "not-a-number "+sentToken)
	assertClean(t, buf.String())
	if s {
		t.Fatal("Stringer was called")
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"regime", "method", "provider", "finish_reason", "class", "status"} {
		if m[k] != "[masked]" {
			t.Fatalf("%s = %v, want [masked]", k, m[k])
		}
	}
}

func TestMaskedValueIsConstantNoLengthOrPrefix(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	l.InfoContext(bg, "request", "regime", sentToken)
	a := buf.String()
	buf.Reset()
	l.InfoContext(bg, "request", "regime", sentToken+strings.Repeat("x", 500))
	b := buf.String()
	strip := func(s string) string {
		var m map[string]any
		_ = json.Unmarshal([]byte(s), &m)
		return fmt.Sprint(m["regime"])
	}
	if strip(a) != "[masked]" || strip(a) != strip(b) {
		t.Fatalf("masked values differ or leak shape: %q %q", strip(a), strip(b))
	}
}

// Valid-SHAPE attempts: look like ids, tokens, tool names, model ids, enums.
var shapeAttempts = []string{sentToken, sentTokenRID, "0123456789abcdef", "h_0123456789abcdef", "case_0123456789ab",
	"pn_285_synth_0001", "gemini-4-argon-SYNTH285", "SAFETY_ZZ285", "stop-285-synth", sentSupplier, sentEmail}

// Every default key that is not a number must refuse every attempt, as a
// string and as a one-element list. Iterating the live policy means a key
// added later is covered automatically.
func TestEveryDefaultKeyRefusesValidShapedSentinels(t *testing.T) {
	p := DefaultPolicy()
	for key := range p.keys {
		for _, attempt := range shapeAttempts {
			l, buf, _ := newTest(t, p)
			l.InfoContext(bg, "request", key, attempt)
			l.InfoContext(bg, "request", key, []string{attempt})
			l.InfoContext(bg, "request", key, slog.StringValue(attempt))
			out := buf.String()
			if strings.Contains(out, attempt) {
				t.Errorf("key %q emitted attempt %q: %s", key, attempt, out)
			}
		}
	}
}

func TestIDKeysAcceptOnlyOpaqueRef(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	rr := RequestRef(sentTokenRID, nil)
	mc, ok := MintedRef("case_0123456789ab")
	if !ok {
		t.Fatal("minted ref rejected")
	}
	l.InfoContext(bg, "request", "request_id", rr, "case", mc, "key_id", mc)
	l.InfoContext(bg, "request", "request_id", "0123456789abcdef", "case", "case_0123456789ab", "key_id", "deadbeefdeadbeef")
	l.InfoContext(bg, "request", "request_id", Ref{})
	out := buf.String()
	assertClean(t, out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.Contains(lines[0], rr.String()) || !strings.Contains(lines[0], `"case":"case_0123456789ab"`) {
		t.Fatalf("refs lost: %s", lines[0])
	}
	if strings.Contains(lines[1], "0123456789abcdef") || strings.Contains(lines[1], "deadbeef") || strings.Count(lines[1], "[masked]") != 3 {
		t.Fatalf("raw ids must be masked: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"request_id":"[masked]"`) {
		t.Fatalf("zero ref must be masked: %s", lines[2])
	}
}

func TestMintedRefRejectsOtherShapes(t *testing.T) {
	for _, s := range []string{"", sentTokenRID, sentEmail, "h_0123456789abcdef", "case_xyz", "0123456789ABCDEF"} {
		if _, ok := MintedRef(s); ok {
			t.Errorf("MintedRef(%q) accepted", s)
		}
	}
}

func TestRegisteredModelsAndTools(t *testing.T) {
	p := DefaultPolicy().AllowModels("gemini-4-argon").AllowTools("what_if", "search_rulings")
	l, buf, _ := newTest(t, p)
	l.InfoContext(bg, "llm call", "model", "gemini-4-argon", "model_id", "gemini-4-argon-001", "tool_calls", []string{"what_if"})
	good := buf.String()
	if strings.Contains(good, "[masked]") || !strings.Contains(good, `"model_id":"gemini-4-argon-001"`) {
		t.Fatalf("registered values lost: %s", good)
	}
	buf.Reset()
	l.InfoContext(bg, "llm call", "model", "gemini-4-argon-SYNTH285", "model_id", "gemini-4-argonX", "tool_calls", []string{"pn_285_synth_0001"})
	l.InfoContext(bg, "llm call", "model", "gemini-4-argon-"+sentToken, "model_id", "gemini-4-argon-123456789")
	out := buf.String()
	if strings.Contains(out, "SYNTH") || strings.Contains(out, "argonX") || strings.Contains(out, "123456789") || strings.Contains(out, "pn_285") {
		t.Fatalf("unregistered shapes leaked: %s", out)
	}
	// Without registration both keys are masked.
	l, buf, _ = newTest(t, DefaultPolicy())
	l.InfoContext(bg, "llm call", "model", "gemini-4-argon", "tool_calls", []string{"what_if"})
	if strings.Contains(buf.String(), "gemini") || strings.Contains(buf.String(), "what_if") {
		t.Fatalf("default must mask model and tool_calls: %s", buf.String())
	}
}

func TestClosedValueSets(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	l.InfoContext(bg, "request", "method", "GET", "provider", "vertex", "finish_reason", "STOP", "regime", "pharma_api")
	l.InfoContext(bg, "request", "method", sentSupplier, "provider", sentBOM, "finish_reason", sentToken, "regime", sentTokenRID)
	out := buf.String()
	assertClean(t, out)
	if strings.Count(out, `[masked]`) != 4 || !strings.Contains(out, `"method":"GET"`) || !strings.Contains(out, `"regime":"pharma_api"`) {
		t.Fatalf("closed sets wrong: %s", out)
	}
}

func TestRouteClosedSet(t *testing.T) {
	// Default: no registered routes, so route is masked (fail closed).
	l, buf, _ := newTest(t, DefaultPolicy())
	l.InfoContext(bg, "request", "route", "PUT /v1/workspaces/{id}/members/{email}/role")
	if !strings.Contains(buf.String(), `"route":"[masked]"`) {
		t.Fatalf("unregistered route must be masked: %s", buf.String())
	}
	p := DefaultPolicy().AllowRoutes("PUT /v1/workspaces/{id}/members/{email}/role", "/")
	l, buf, _ = newTest(t, p)
	l.InfoContext(bg, "request", "route", "PUT /v1/workspaces/{id}/members/{email}/role")
	l.InfoContext(bg, "request", "route", "PUT /v1/workspaces/ws_x/members/"+sentEmail+"/role")
	out := buf.String()
	assertClean(t, out)
	if strings.Count(out, `[masked]`) != 1 {
		t.Fatalf("raw path must be masked, pattern kept: %s", out)
	}
}

func TestMessageClosedAllowlist(t *testing.T) {
	l, buf, h := newTest(t, DefaultPolicy())
	l.InfoContext(bg, sentMsg)
	l.InfoContext(bg, "panic: runtime error index out of range [3] with length 2") // regex-shaped, not known
	l.InfoContext(bg, "request")
	l.InfoContext(bg, "request\n"+sentSupplier)
	out := buf.String()
	assertClean(t, out)
	if strings.Count(out, `"msg":"[message masked]"`) != 3 || !strings.Contains(out, `"msg":"request"`) {
		t.Fatalf("message allowlist wrong: %s", out)
	}
	if h.Stats().MaskedMessages != 3 {
		t.Fatalf("stats %+v", h.Stats())
	}
}

func TestDebugContentMessageNotKnown(t *testing.T) {
	if DefaultPolicy().KnownMessage("llm call content") {
		t.Fatal("the raw-content debug message must not be allowlisted")
	}
}

func TestOperationalFieldsKept(t *testing.T) {
	p := DefaultPolicy().AllowModels("gemini-4-argon").AllowTools("what_if")
	l, buf, _ := newTest(t, p)
	rid := RequestRef("", func() string { return "0123456789abcdef" })
	l.InfoContext(bg, "llm call", "provider", "vertex", "model", "gemini-4-argon", "latency_ms", int64(12), "model_id", "gemini-4-argon-001",
		"input_tokens", 10, "output_tokens", 5, "tool_calls", []string{"what_if"}, "finish_reason", "STOP", "request_id", rid,
		"spent_usd", 1.25, "cap_usd", 20.0, "status", 200)
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"provider", "model", "latency_ms", "model_id", "input_tokens", "output_tokens", "tool_calls", "finish_reason", "request_id", "spent_usd", "cap_usd", "status"} {
		if m[k] == nil || m[k] == "[masked]" {
			t.Fatalf("operational key %s lost: %v", k, m[k])
		}
	}
	if m["request_id"] != "0123456789abcdef" {
		t.Fatalf("server-minted id must not be re-hashed: %v", m["request_id"])
	}
	if _, ok := m["masked_attrs"]; ok {
		t.Fatal("nothing should have been masked")
	}
	if m["time"] == nil || m["level"] != "INFO" {
		t.Fatalf("standard attrs lost: %v", m)
	}
}

func TestRulePanicDropsRecordFailClosed(t *testing.T) {
	p := DefaultPolicy().Allow("boom", func(slog.Value) (slog.Value, bool) { panic(sentSupplier) })
	l, buf, h := newTest(t, p)
	l.ErrorContext(bg, "request", "status", 500, "boom", sentBOM)
	out := buf.String()
	assertClean(t, out)
	if !strings.Contains(out, `"msg":"log record dropped"`) || !strings.Contains(out, "sanitizer_failure") || strings.Contains(out, `"status"`) {
		t.Fatalf("want constant fallback only: %s", out)
	}
	if h.Stats().DroppedRecords != 1 {
		t.Fatalf("stats %+v", h.Stats())
	}
}

type countingHandler struct {
	mu     sync.Mutex
	writes int
	seen   []string
	fail   bool
}

func (c *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (c *countingHandler) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes++
	c.seen = append(c.seen, r.Message)
	r.Attrs(func(a slog.Attr) bool { c.seen = append(c.seen, a.Key, a.Value.String()); return true })
	if c.fail {
		return errors.New("downstream: " + sentSupplier)
	}
	return nil
}
func (c *countingHandler) WithAttrs(a []slog.Attr) slog.Handler { return c }
func (c *countingHandler) WithGroup(string) slog.Handler        { return c }

func TestInnerFailureNoRawRetry(t *testing.T) {
	c := &countingHandler{fail: true}
	nh, err := NewHandler(c, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	l := slog.New(nh)
	l.InfoContext(bg, "request", sentKeyName, sentBOM)
	if c.writes != 1 {
		t.Fatalf("inner written %d times, want exactly 1 (no raw retry)", c.writes)
	}
	assertClean(t, strings.Join(c.seen, "|"))
}

func TestInnerPanicDoesNotPropagateOrRetry(t *testing.T) {
	h, herr := NewHandler(panicHandler{}, DefaultPolicy())
	if herr != nil {
		t.Fatal(herr)
	}
	if err := h.Handle(bg, slog.NewRecord(timeZero(), slog.LevelInfo, "request", 0)); err != nil && strings.Contains(err.Error(), "Zzyzx") {
		t.Fatal("downstream text propagated")
	} else if err != nil {
		t.Logf("returned: %v", err) // an error is acceptable, a panic is not
	}
}

type panicHandler struct{}

func (panicHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (panicHandler) Handle(context.Context, slog.Record) error { panic(sentSupplier) }
func (p panicHandler) WithAttrs([]slog.Attr) slog.Handler      { return p }
func (p panicHandler) WithGroup(string) slog.Handler           { return p }

func TestNewHandlerRewrapRules(t *testing.T) {
	inner := slog.NewJSONHandler(&strings.Builder{}, nil)
	p := DefaultPolicy()
	h, err := NewHandler(inner, p)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := NewHandler(h, p); err != nil || again != h {
		t.Fatalf("same policy must return the same handler: %v", err)
	}
	if _, err := NewHandler(h, DefaultPolicy().AllowRoutes("GET /x")); !errors.Is(err, ErrAlreadyWrapped) {
		t.Fatalf("a different policy must be rejected, got %v", err)
	}
	if _, err := NewHandler(nil, p); err == nil {
		t.Fatal("nil inner accepted")
	}
	if _, err := NewHandler(inner, nil); err == nil {
		t.Fatal("nil policy accepted")
	}
}

func TestPolicyIsClonedAtConstruction(t *testing.T) {
	p := DefaultPolicy()
	l, buf, _ := newTest(t, p)
	p.Allow(sentKeyName, Number()) // too late: must not affect the live handler
	l.InfoContext(bg, "request", sentKeyName, 1)
	assertClean(t, buf.String())
}

func TestConcurrentUse(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				l.With("status", j).InfoContext(bg, "request", sentKeyName, sentBOM)
			}
		}()
	}
	wg.Wait()
	assertClean(t, buf.String())
}
