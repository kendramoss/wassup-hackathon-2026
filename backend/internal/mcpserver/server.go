// Package mcpserver is the Model Context Protocol server for the Unde API
// (docs/MCP.md): Streamable HTTP, stateless, served at POST /mcp.
//
// It holds no business logic. Every tool is a thin wrapper over one /v1
// route: the tool validates its input against a tight JSON schema, builds the
// REST request, and dispatches it IN-PROCESS to the API handler it was given
// (the whole middleware chain), with only the caller's API key, client
// address and, for anonymous callers, a case access key. So a tool call gets
// exactly what the REST call gets: the API-key check and daily quota, the
// route's rate-limit class, the global spend cap, case isolation, the audit
// row and the request log line. The admin token and user token never reach
// the API from here.
//
// Stateless: the server keeps no MCP session, so any instance can answer any
// request (Cloud Run runs several).
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configure the server.
type Options struct {
	// API is the REST API's root handler. Tools call it in-process with
	// paths under /v1/.
	API http.Handler
	// Families are the product families the API has templates for; they
	// become the enum of every "family" input. Empty: any short string.
	Families []string
	// RulingSearch reports that GET /v1/rulings?q= is served; without it the
	// search_rulings tool is not listed.
	RulingSearch bool
	// Version is reported in the MCP initialize result.
	Version string
}

// maxBody caps one MCP request body. Tool inputs are small (the largest is
// 12,000 characters of facts or a product of a few hundred rows).
const maxBody = 1 << 20

// New returns the Streamable HTTP handler for POST /mcp.
func New(o Options) http.Handler {
	s := NewServer(o)
	// DNS-rebinding protection is off: it refuses a loopback connection whose
	// Host is not loopback, which is a local desktop server's risk, not this
	// one's. The backend is private behind the frontend proxy deployed, and
	// locally its whole /v1 API is reachable the same way anyway; keeping the
	// check would only risk refusing the proxy's own requests.
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, MaxRequestBodyBytes: maxBody, DisableLocalhostProtection: true})
	return guard(h)
}

// NewServer builds the MCP server with its tools, resources and prompt.
func NewServer(o Options) *mcp.Server {
	v := o.Version
	if v == "" {
		v = "dev"
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "unde", Title: "Unde Labs", Version: v}, &mcp.ServerOptions{Instructions: instructions})
	t := tools{api: o.API, families: o.Families}
	t.register(s, o.RulingSearch)
	t.registerResources(s)
	t.registerPrompts(s)
	return s
}

const instructions = `Unde predicts the country of origin U.S. Customs (CBP) would likely assign to a product under the substantial-transformation test, citing earlier CBP rulings, and searches production moves that could change it.
Typical flow: extract_product (optional, turns notes into parts) -> predict_origin (creates a case) -> plan_production_moves or compare_target_countries -> export_ruling_request_draft.
Results are predictions, not rulings. Always tell the user to confirm with a licensed customs broker or a CBP binding ruling.
Without an API key, predict_origin returns a case_access_key: pass it to every later call on that case.`

// guard maps the caller's credentials before the MCP handler sees the
// request. "Authorization: Bearer op_..." becomes X-API-Key (deployed, the
// Authorization header otherwise holds the platform's identity token, which
// is ignored). The admin token is refused outright, and the user token and
// visitor ID are dropped: an MCP caller is an API key or anonymous. Without
// X-Client-IP (a direct local call) the connection address is used, so
// anonymous callers get their own rate-limit bucket.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Token") != "" {
			writeHTTPError(w, http.StatusForbidden, "forbidden", "the admin token is not accepted on /mcp; use an API key")
			return
		}
		key := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if tok, ok := bearerKey(r.Header.Get("Authorization")); ok {
			if key != "" && key != tok {
				writeHTTPError(w, http.StatusBadRequest, "invalid_input", "X-API-Key and the Authorization bearer key differ; send one")
				return
			}
			key = tok
		}
		r = r.Clone(r.Context())
		for _, h := range []string{"Authorization", "X-API-Key", "X-User-Token", "X-Visitor-ID", "Cookie"} {
			r.Header.Del(h)
		}
		if key != "" {
			r.Header.Set("X-API-Key", key)
		}
		if strings.TrimSpace(r.Header.Get("X-Client-IP")) == "" {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			r.Header.Set("X-Client-IP", host)
		}
		next.ServeHTTP(w, r)
	})
}

// bearerKey returns the API key in an "Authorization: Bearer op_..." header.
func bearerKey(h string) (string, bool) {
	scheme, tok, ok := strings.Cut(strings.TrimSpace(h), " ")
	tok = strings.TrimSpace(tok)
	if !ok || !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(tok, "op_") {
		return "", false
	}
	return tok, true
}

func writeHTTPError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": msg})
}

// apiCall is one in-process REST call.
type apiCall struct {
	method, path string
	query        url.Values
	body         any    // JSON-encoded when not nil
	access       string // case access key (anonymous callers): sent as X-Visitor-ID
}

// apiResult is the REST answer: the status and the decoded JSON body.
type apiResult struct {
	status int
	body   map[string]any
}

func (r apiResult) ok() bool { return r.status >= 200 && r.status < 300 }

// call dispatches c to the API handler with the caller's identity headers
// only (from is the MCP request's header, already mapped by guard).
func (t tools) call(ctx context.Context, from http.Header, c apiCall) (apiResult, error) {
	if !strings.HasPrefix(c.path, "/v1/") {
		return apiResult{}, fmt.Errorf("mcpserver: %q is not an API path", c.path)
	}
	var body io.Reader = http.NoBody
	if c.body != nil {
		b, err := json.Marshal(c.body)
		if err != nil {
			return apiResult{}, err
		}
		body = bytes.NewReader(b)
	}
	u := c.path
	if len(c.query) > 0 {
		u += "?" + c.query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, c.method, u, body)
	if err != nil {
		return apiResult{}, err
	}
	req.RequestURI = ""
	req.Host = "mcp.local"
	req.Header.Set("Accept", "application/json")
	if c.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if from != nil {
		if ip := strings.TrimSpace(from.Get("X-Client-IP")); ip != "" {
			req.Header.Set("X-Client-IP", ip)
			req.RemoteAddr = net.JoinHostPort(ip, "0")
		}
		if k := from.Get("X-API-Key"); k != "" {
			req.Header.Set("X-API-Key", k)
		}
	}
	if c.access != "" && req.Header.Get("X-API-Key") == "" {
		req.Header.Set("X-Visitor-ID", c.access)
	}
	rec := &recorder{header: http.Header{}}
	t.api.ServeHTTP(rec, req)
	if rec.code == 0 {
		rec.code = http.StatusOK
	}
	out := apiResult{status: rec.code}
	if rec.buf.Len() > 0 {
		if err := json.Unmarshal(rec.buf.Bytes(), &out.body); err != nil {
			out.body = map[string]any{"code": "internal", "message": "the API answered with something that is not a JSON object"}
		}
	}
	if out.body == nil {
		out.body = map[string]any{}
	}
	if ra := rec.header.Get("Retry-After"); ra != "" && !out.ok() {
		out.body["retry_after"] = ra
	}
	return out, nil
}

// recorder collects an in-process response.
type recorder struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}
func (r *recorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.buf.Write(b)
}
