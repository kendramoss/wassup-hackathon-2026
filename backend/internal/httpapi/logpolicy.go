package httpapi

import (
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// LogToolNames are the tool names the model may call (the case agent's and
// the verifier's). The log policy registers exactly these for the tool_calls
// field; a name the model invents is masked, not logged. A test keeps this
// list equal to the tools declared in internal/caseagent and internal/verifier.
var LogToolNames = []string{"get_case", "get_case_facts", "search_rulings", "read_ruling", "what_if"}

// HealthCheckNames are the names of the deep-health checks wired in main.
var HealthCheckNames = []string{"case_store", "model"}

// LogPolicy is the COMPLETE allowlist policy for the server's log handler
// (#285): the logsafe operational defaults plus the closed sets this package
// knows (every route pattern, tool name, health check name, audit action and
// failure class) and the configured model ids. It must be finished here,
// before the one logsafe.NewHandler call: the handler takes a copy, and a
// policy changed after wrapping is not seen. Startup keys belong to the
// caller (cmd/server), which adds them to the result BEFORE wrapping.
//
// models are the configured model ids (ENGINE_MODEL, ROUTER_MODEL, ...). A
// provider-reported version of one of them ("<id>-<digits>") is accepted; any
// other value is masked.
func LogPolicy(models ...string) *logsafe.Policy {
	p := logsafe.DefaultPolicy()
	p.AllowRoutes(RoutePatterns()...)
	p.AllowModels(models...)
	p.AllowTools(LogToolNames...)
	p.Allow("intent_seq", logsafe.Number())
	p.Allow("check", logsafe.OneOf(HealthCheckNames...))
	// What an audit write was doing when it failed (logFail's first argument).
	p.Allow("what", logsafe.OneOf("intent", "completion", "denial", "single", "auth denial"))
	actions := audit.Actions()
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = string(a)
	}
	p.Allow("action", logsafe.OneOf(names...))
	// logsafe's failure classes plus the two audit write classes auditErrClass
	// returns.
	classes := []string{"invalid_event", "store"}
	for _, c := range logsafe.AllClasses() {
		classes = append(classes, string(c))
	}
	p.Allow("class", logsafe.OneOf(classes...))
	return p
}
