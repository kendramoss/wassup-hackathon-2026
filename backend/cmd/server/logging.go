package main

import (
	"io"
	"log"
	"log/slog"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// newLogger builds the server's ROOT logger and the *log.Logger for
// http.Server.ErrorLog (#285). Both write through ONE logsafe allowlist
// handler over JSON on w, built from a policy that is complete before the one
// logsafe.NewHandler call: the closed route, model, tool and startup sets come
// from the real route table and the loaded config, and nothing is added after.
//
// Nothing here reads a key, a secret or the audit setting: the log policy
// does not depend on audit, and AUDIT=off changes nothing in it. On error the
// caller must not log through an unprotected logger.
func newLogger(w io.Writer, cfg config.Config) (*slog.Logger, *log.Logger, error) {
	h, err := logsafe.NewHandler(slog.NewJSONHandler(w, nil), logPolicy(cfg))
	if err != nil {
		return nil, nil, err
	}
	return slog.New(h), logsafe.ErrorLog(h), nil
}

// logPolicy is the complete allowlist policy of the server process: the
// httpapi policy (routes, tools, check names, audit actions, failure classes,
// the configured model ids) plus the startup keys main.go and audit.go log.
//
// Every startup value is a number or a member of a closed set built from the
// loaded config or from a constant of the code; none can carry request text.
// The config summary is accepted only as the exact string cfg.String()
// produces (a test asserts it holds no secret).
func logPolicy(cfg config.Config) *logsafe.Policy {
	p := httpapi.LogPolicy(configuredModels(cfg)...)
	for _, k := range startupNumberKeys {
		p.Allow(k, logsafe.Number())
	}
	for k, vals := range startupValues(cfg) {
		p.Allow(k, logsafe.OneOf(vals...))
	}
	p.Allow("views", logsafe.StringList(logsafe.OneOf(verifier.ViewSkeptic, verifier.ViewProduct)))
	p.Allow("collections", logsafe.StringList(logsafe.OneOf(store.VerdictCollection, store.EngineCollection)))
	return p
}

// configuredModels lists every model id the config can send a request to.
func configuredModels(cfg config.Config) []string {
	var out []string
	for _, m := range []string{cfg.EngineModel, cfg.RouterModel, cfg.CaseAgentModel, cfg.VerifierModel, cfg.FallbackEngineModel, japanModel} {
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// startupNumberKeys are the numeric (count, size, duration, bool) attributes of
// the startup lines.
var startupNumberKeys = []string{
	"max_in_flight", "usd", "primary_attempts", "fast_path", "precedents", "low", "high", "floor", "max_per_search",
	"moves_per_search", "fallback_rulings", "fallback_subheadings", "ttl", "timeout", "retention_days", "deleted",
	"subfamilies", "global",
}

// startupValues maps each string-valued startup key to the closed set of values
// it may carry.
func startupValues(cfg config.Config) map[string][]string {
	return map[string][]string{
		"summary":            {cfg.String()},
		"port":               {cfg.Port},
		"llm_provider":       {cfg.LLMProvider, config.ProviderVertex, config.ProviderFake},
		"gap_after_overload": {cfg.LLMCallGap.String()},
		"cool_down":          {cfg.LLMCoolDown.String()},
		"location":           {cfg.FallbackEngineLocation},
		"facts_dir":          {cfg.FactsDir},
		"prompt":             {engine.TextileVersion, engine.TextileVersion2, engine.USMCAVersion, engine.USMCAVersion2, engine.USMCAVersion5, engine.PharmaVersion, nearmiss.PromptVersion, engine.PromptVersion},
		"map_prompt":         {nearmiss.MapPromptVersion},
		"source":             {duty.DefaultHTSBaseURL, "cross"},
		"collection":         {store.RateLimitCollection},
		"index":              {cfg.HTSIndex, cfg.PartHTSIndex},
		"audience":           {cfg.TeamsAudience},
		"project":            {cfg.GCPProject},
		"database":           {cfg.FirestoreDatabase},
		"audit":              {cfg.AuditMode},
		"actor_key_id":       {cfg.AuditActorKeyID},
		// Which kind of store backs a feature: fixed words, not Go type names.
		"store":       {config.CasesFirestore, config.CasesFile, config.CasesMemory, cfg.AuditStore},
		"workspaces":  {config.CasesFirestore, config.CasesMemory},
		"session_ttl": {cfg.SSOSessionTTL.String()},
	}
}
