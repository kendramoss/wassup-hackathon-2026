package logsafe

import (
	"log/slog"
	"maps"
	"math"
	"regexp"
	"strings"
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

// productionConfigWarnings are the exact texts config.Config.Warnings can
// emit. Keeping them in the log allowlist means a real warning is printed,
// while an arbitrary string under "warning" is still dropped (#420).
var productionConfigWarnings = []string{
	"STRICT_AUTH is off: a visitor ID alone is accepted as an anonymous principal; Cloud Run IAM and CASE_OWNERSHIP remain the boundaries",
	"REQUIRE_BEARER is off: the backend accepts proxied requests without a bearer token; verify Cloud Run header pass-through before enabling it",
	"AUDIT_ACTOR_KEY is unset (or AUDIT=off): no audit trail is written; this is an explicit product choice, not certification",
}

// Rule accepts or rejects one value for one key. It receives values that are
// never LogValuers (except logsafe's own). It returns the value to emit.
type Rule func(slog.Value) (slog.Value, bool)

var (
	enumRE    = regexp.MustCompile(`^[A-Za-z0-9._:/+-]{1,80}$`)
	modelRE   = regexp.MustCompile(`^(gemini|gemma|text-embedding|textembedding|embedding|claude|gpt|fake)[A-Za-z0-9._:/+-]{0,60}$`)
	versionRE = regexp.MustCompile(`^[0-9]{1,8}$`)
	toolRE    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)
	idRE      = regexp.MustCompile(`^((case|ws|key)_[0-9a-f]{8,32}|[0-9a-f]{16}|h_[0-9a-f]{16})$`)
)

// Enum accepts a short string (a SHAPE helper, NOT used by any default key) of letters, digits and ._:/+- only (no '@', no
// spaces). It is a shape limit, not a privacy proof: it narrows what an
// allowed key can carry. Prefer OneOf for small closed sets.
func Enum() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && len(v.String()) <= maxEnumLen && enumRE.MatchString(v.String())
	}
}

// ModelID accepts a model identifier (a SHAPE helper, not a default allow) that starts with a known model family
// (gemini, gemma, text-embedding, claude, gpt, fake) followed by the usual
// version characters. Extend with Allow for other families.
func ModelID() Rule {
	return func(v slog.Value) (slog.Value, bool) {
		return v, v.Kind() == slog.KindString && modelRE.MatchString(v.String())
	}
}

// ModelSet accepts exactly the registered model ids, or a registered id plus
// "-" and 1..8 digits (a provider-reported version suffix). Nothing else.
func ModelSet(ids ...string) Rule {
	set := map[string]bool{}
	for _, s := range ids {
		set[s] = true
	}
	return func(v slog.Value) (slog.Value, bool) {
		if v.Kind() != slog.KindString {
			return v, false
		}
		s := v.String()
		if set[s] {
			return v, true
		}
		if i := strings.LastIndexByte(s, '-'); i > 0 && set[s[:i]] && versionRE.MatchString(s[i+1:]) {
			return v, true
		}
		return v, false
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

// ID is a SHAPE helper (NOT used by any default key: raw ids use the opaque Ref): case_/ws_/key_ plus hex, a
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
	"USMCA engine on", "api key disabled", "api key failures limited", "api key issued", "api key quota exceeded", "api key quota not counted",
	"audit export stopped", "audit retention purge", "audit retention purge failed", "audit trail NOT enabled: no events are recorded and no audit evidence is produced (set AUDIT_ACTOR_KEY, or AUDIT=on with it)",
	"audit trail is in memory: it is lost on restart and not shared between instances", "audit trail on", "audit write failed",
	"auto-accept bars by subfamily", "cache blobs encrypted", "calibrated confidence on", "cases persisted in Firestore", "cases persisted",
	"clarify failed; staying needs_review", "client daily model budget reached", "config", "corpus loaded", "daily model budget reached",
	"daily model spend cap", "engine fallback on", "fatal", "following near-miss advice", "health check failed",
	"hts reasons failed; candidates without reasons", "hts suggest on",
	"parts hts reasons failed; ruling candidates without reasons", "parts hts suggest on",
	"parts hts suggest off: index not loaded", "placement guard on", "parts hts suggest: no product index, so no code for the good",
	"import: family check failed; family left as given", "internal error", "listening", "live MFN duty rates on",
	"live MFN lookup failed", "live MFN lookup failed; no rate filled", "llm call failed", "llm call",
	"llm call sizes", "model bad response", "model calls paced", "model unavailable",
	"money prefill: hts suggestion failed; no duty filled", "near-miss advice on", "pharma engine on",
	"precedent-check panel on", "production config", "rate counters not pruned", "rate limits counted in fixed windows in memory", "rate limits shared in Firestore",
	"request", "retention on: workspace policies and tombstones", "router failed; scope unknown", "shared rate limiter unavailable; using this instance's limits",
	"sign-in token refused", "specialist engine failed; provisional general prediction needs review",
	"sso error", "sso on: OIDC and SAML sign-in", "sso provider returned an error", "sso sign-in refused",
	"sso sign-in refused: member of another workspace",
	"teams on: Google sign-in and workspaces", "textile engine on",
	"verifier verdicts and engine predictions cached in Firestore", "verifier verdicts and engine predictions cached",
	"watch failed", "workspace usage metering on", "workspace usage unavailable; not enforcing the budget", msgStdLog,
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
	// Closed value sets only. model, model_id and tool_calls have no default
	// (register them at wiring with AllowModels / AllowTools); check has no
	// call site and no default; ids accept only the opaque Ref.
	p.keys["finish_reason"] = OneOf("STOP", "MAX_TOKENS", "SAFETY", "RECITATION", "LANGUAGE", "OTHER", "BLOCKLIST",
		"PROHIBITED_CONTENT", "SPII", "MALFORMED_FUNCTION_CALL", "IMAGE_SAFETY", "UNEXPECTED_TOOL_CALL",
		"FINISH_REASON_UNSPECIFIED", "stop", "length", "tool_calls", "content_filter", "function_call")
	p.keys["regime"] = OneOf("substantial_transformation", "textile_102_21", "usmca_part102", "pharma_api", "unknown")
	p.keys["provider"] = OneOf("vertex", "fake", "litellm", "litellm-eval")
	p.keys["method"] = OneOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS")
	for _, k := range []string{"request_id", "case", "key_id"} {
		p.keys[k] = refRule
	}
	classes := make([]string, len(allClasses))
	for i, c := range allClasses {
		classes[i] = string(c)
	}
	p.keys["class"] = OneOf(classes...)
	p.keys["reason"] = OneOf(reasonSanitizer)
	// #420 startup disclosure: only the three fixed warning texts may be
	// logged under "warning"; arbitrary text is never printed by the policy.
	p.keys["warning"] = OneOf(productionConfigWarnings...)
	p.keys["err"] = errRule
	p.keys["route"] = routeRule(p.routes)
	return p
}

// Allow registers (or replaces) the rule for key.
func (p *Policy) Allow(key string, r Rule) *Policy { p.keys[key] = r; return p }

// AllowModels registers the closed set of configured model ids for the model
// and model_id keys (see ModelSet). Call once at wiring with the configured
// models; until then both keys are masked.
func (p *Policy) AllowModels(ids ...string) *Policy {
	p.keys["model"], p.keys["model_id"] = ModelSet(ids...), ModelSet(ids...)
	return p
}

// AllowTools registers the closed set of tool names for the tool_calls key.
func (p *Policy) AllowTools(names ...string) *Policy {
	p.keys["tool_calls"] = StringList(OneOf(names...))
	return p
}

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
