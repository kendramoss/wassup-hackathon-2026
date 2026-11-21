package httpapi

import (
	"net/http"
	"slices"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/mcpserver"
)

// mcp serves the MCP endpoint (docs/MCP.md). Each tool call is dispatched
// in-process to the /v1 route it wraps, through the whole middleware chain
// (*root, set once NewHandler has built it), with the caller's API key and
// client address only. So a tool call gets exactly the REST route's API-key
// check and quota, rate-limit class, spend cap, case isolation, audit row and
// request log line. The admin token and the user token are never forwarded.
func (a *api) mcp(root *http.Handler, version string) http.HandlerFunc {
	families := make([]string, 0, len(a.d.Templates))
	for f := range a.d.Templates {
		families = append(families, f)
	}
	slices.Sort(families)
	h := mcpserver.New(mcpserver.Options{
		API:          http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { (*root).ServeHTTP(w, r) }),
		Families:     families,
		RulingSearch: a.d.RulingSearch != nil,
		MediaExtract: a.d.MediaExtractor != nil,
		PartsHTS:     a.d.PartsHTS != nil,
		Version:      version,
		// The MCP guard resolves the address with the same trusted-proxy
		// rule as clientKey, so a direct caller's header cannot pick the
		// bucket of its tool calls (#479).
		ClientAddress: func(remote, ip string) string { return clientAddressWithProxy(remote, ip, a.d.TrustedProxyCIDRs) },
	})
	return h.ServeHTTP
}
