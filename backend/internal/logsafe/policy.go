package logsafe

import (
	"log/slog"
	"maps"
	"math"
	"regexp"
)

const (
	masked          = "[masked]"
	msgMasked       = "[message masked]"
	msgDropped      = "log record dropped"
	msgStdLog       = "http server log line"
	keyMaskedAttrs  = "masked_attrs"
	reasonSanitizer = "sanitizer_failure"
	maxListElems    = 32
	maxEnumLen      = 80
)

// Rule accepts or rejects one value for one key. It receives values that are
// never LogValuers (except logsafe's own). It returns the value to emit.
type Rule func(slog.Value) (slog.Value, bool)

var (
	enumRE  = regexp.MustCompile(`^[A-Za-z0-9._:/+-]{1,80}$`)
	modelRE = regexp.MustCompile(`^(gemini|gemma|text-embedding|textembedding|embedding|claude|gpt|fake)[A-Za-z0-9._:/+-]{0,60}$`)
	toolRE  = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
	idRE    = regexp.MustCompile(`^((case|ws|key)_[0-9a-f]{8,32}|[0-9a-f]{16}|h_[0-9a-f]{16})$`)
)

// Enum accepts a short string of letters, digits and ._:/+- only (no '@', no
// spaces). It is a shape limit, not a privacy proof: it narrows what an
// allowed key can carry. Prefer OneOf for small closed sets.
func Enum() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && len(v.String()) <= maxEnumLen && enumRE.MatchString(v.String())
	}
}

// ModelID accepts a model identifier that starts with a known model family
// (gemini, gemma, text-embedding, claude, gpt, fake) followed by the usual
// version characters. Extend with Allow for other families.
func ModelID() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && modelRE.MatchString(v.String())
	}
}

// ToolName accepts a lower-case snake_case identifier of 2..41 characters.
func ToolName() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && toolRE.MatchString(v.String())
	}
}

// OneOf accepts exactly the listed strings.
func OneOf(vals ...string) Rule {
	set := map[string]bool{}
	for _, s := range vals {
		set[s] = true
	}
	return func(v slog.Value) (slog.Value, bool) { return v, v.Kind() == slog.KindString && set[v.String()] }
}

// ID accepts a server-minted identifier form: case_/ws_/key_ plus hex, a
// 16-hex request id, or the h_<16 hex> form of RequestID.
func ID() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && idRE.MatchString(v.String())
	}
}

// Number accepts numeric, bool, duration and time kinds. A number carries no
// text; the KEY decides whether a number may be logged at all.
func Number() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		switch v.Kind() {
		case slog.KindInt64, slog.KindUint64, slog.KindBool, slog.KindDuration, slog.KindTime:
			return v, true
		case slog.KindFloat64:
			f := v.Float64()
			return v, !math.IsNaN(f) && !math.IsInf(f, 0)
		}
		return v, false
	}
}

// StringList accepts a []string of at most 32 elements, each accepted by elem.
func StringList(elem Rule) Rule {
	return func(v slog.Value) (slog.Value, bool) {
		if v.Kind() != slog.KindAny {
			return v, false
		}
		l, ok := v.Any().([]string)
		if !ok || len(l) > maxListElems {
			return v, false
		}
		out := make([]string, len(l))
		for i, s := range l {
			if _, ok := elem(slog.StringValue(s)); !ok {
				return v, false
			}
			out[i] = s
		}
		return slog.AnyValue(out), true
	}
}

func errRule(v slog.Value) (slog.Value, bool) {
	switch v.Kind() {
	case slog.KindLogValuer:
		if ev, ok := v.Any().(errValue); ok {
			return errGroup(ev.err), true
		}
	case slog.KindAny:
		if e, ok := v.Any().(error); ok {
			return errGroup(e), true
		}
	}
	return v, false
}

func routeRule(routes map[string]bool) Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && routes[v.String()]
	}
}

// Policy is the closed allowlist: keys with value rules, group names, known
// messages and registered route patterns. Build it, then hand it to
// NewHandler, which takes a copy.
type Policy struct {
	keys     map[string]Rule
	groups   map[string]bool
	messages map[string]bool
	routes   map[string]bool
}

// knownMessages are the literal log messages of the current code base, minus
// "llm call content" (raw prompts and answers), which is deliberately absent.
var knownMessages = []string{
	"API keys on",
	"STRICT_AUTH is on but neither TEAMS nor API_KEYS is: only the admin token can reach private case routes",
	"USMCA engine on", "api key disabled", "api key issued", "api key quota exceeded", "api key quota not counted",
	"auto-accept bars by subfamily", "cases persisted in Firestore", "cases persisted",
	"clarify failed; staying needs_review", "config", "corpus loaded", "daily model budget reached",
	"daily model spend cap", "engine fallback on", "fatal", "following near-miss advice", "health check failed",
	"hts reasons failed; candidates without reasons", "hts suggest on",
	"import: family check failed; family left as given", "internal error", "listening", "live MFN duty rates on",
	"live MFN lookup failed", "live MFN lookup failed; no rate filled", "llm call failed", "llm call",
	"llm call sizes", "model bad response", "model calls paced", "model unavailable",
	"money prefill: hts suggestion failed; no duty filled", "near-miss advice on", "pharma engine on",
	"precedent-check panel on", "rate limits counted in fixed windows in memory", "rate limits shared in Firestore",
	"request", "router failed; scope unknown", "shared rate limiter unavailable; using this instance's limits",
	"sign-in token refused", "specialist engine failed; provisional general prediction needs review",
	"teams on: Google sign-in and workspaces", "textile engine on",
	"verifier verdicts and engine predictions cached in Firestore", "verifier verdicts and engine predictions cached",
	"watch failed", msgStdLog,
}

// DefaultPolicy is the operational allowlist: status, latency, token counts,
// cost, trusted model metadata, server ids, closed classes. Startup/config
// keys are NOT included; the wiring unit registers them with Allow.
func DefaultPolicy() *Policy {
	p := &Policy{keys: map[string]Rule{}, groups: map[string]bool{"err": true}, messages: map[string]bool{}, routes: map[string]bool{}}
	for _, m := range knownMessages {
		p.messages[m] = true
	}
	for _, k := range []string{"latency_ms", "status", "input_tokens", "output_tokens", "spent_usd", "cap_usd", "used", "quota",
		"daily_quota", "system_chars", "messages", "text_chars", "http_status", keyMaskedAttrs} {
		p.keys[k] = Number()
	}
	for _, k := range []string{"finish_reason", "check", "regime"} {
		p.keys[k] = Enum()
	}
	p.keys["model"], p.keys["model_id"] = ModelID(), ModelID()
	p.keys["provider"] = OneOf("vertex", "fake", "litellm-eval")
	p.keys["method"] = OneOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS")
	for _, k := range []string{"request_id", "case", "key_id"} {
		p.keys[k] = ID()
	}
	p.keys["tool_calls"] = StringList(ToolName())
	classes := make([]string, len(allClasses))
	for i, c := range allClasses {
		classes[i] = string(c)
	}
	p.keys["class"] = OneOf(classes...)
	p.keys["reason"] = OneOf(reasonSanitizer)
	p.keys["err"] = errRule
	p.keys["route"] = routeRule(p.routes)
	return p
}

// Allow registers (or replaces) the rule for key.
func (p *Policy) Allow(key string, r Rule) *Policy { p.keys[key] = r; return p }

// AllowGroup registers a group name.
func (p *Policy) AllowGroup(name string) *Policy { p.groups[name] = true; return p }

// AllowMessage registers a known-source log message.
func (p *Policy) AllowMessage(m string) *Policy { p.messages[m] = true; return p }

// AllowRoutes registers mux patterns for the route key. With none registered
// the route key is always masked (fail closed).
func (p *Policy) AllowRoutes(patterns ...string) *Policy {
	for _, s := range patterns {
		p.routes[s] = true
	}
	return p
}

// KnownMessage reports whether m is an allowed message.
func (p *Policy) KnownMessage(m string) bool { return p.messages[m] }

func (p *Policy) clone() *Policy {
	c := &Policy{keys: maps.Clone(p.keys), groups: maps.Clone(p.groups), messages: maps.Clone(p.messages), routes: maps.Clone(p.routes)}
	c.keys["route"] = routeRule(c.routes)
	return c
}
