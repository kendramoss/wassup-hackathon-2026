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
// the API from here: this guard drops their headers, and the API's withUser
// middleware also skips /mcp, so a verified identity cannot ride into a tool
// call through the request context (issue #385). As defence in depth, every
// tool request is built on a context that carries none of the outer
// request's values (see call).
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
	// MediaExtract reports that POST /v1/products/extract-media is served;
	// without it extract_product_from_media is not listed.
	MediaExtract bool
	// PartsHTS reports that POST /v1/products/parts-hts-suggest is served;
	// without it suggest_part_hts is not listed.
	PartsHTS bool
	// CanadaMarking reports that POST /v1/rules/canada-marking is served;
	// without it check_canada_marking is not listed.
	CanadaMarking bool
	// EUOrigin and UKOrigin report that POST /v1/rules/eu-origin and
	// POST /v1/rules/uk-origin are served; without them check_eu_origin and
	// check_uk_origin are not listed.
	EUOrigin bool
	UKOrigin bool
	// ATIGA reports that POST /v1/atiga/check is served; without it
	// check_atiga_preference is not listed.
	ATIGA bool
	// ADCVD reports that POST /v1/adcvd/check is served; without it
	// check_adcvd_exposure is not listed.
	ADCVD bool
	// Version is reported in the MCP initialize result.
	Version string
	// ClientAddress resolves the outer request's connection address and
	// X-Client-IP once, before any tool call. httpapi supplies the
	// TRUSTED_PROXY_CIDRS rule; without one the connection address wins, so
	// a direct caller cannot choose the rate-limit bucket of its tool calls
	// (#479). The resolved value is the only client address tools see.
	ClientAddress func(remoteAddr, clientIP string) string
}

// maxBody caps one MCP request body: room for extract_product_from_media's
// 8 MB of files in base64 (about 11.2 MB) and the JSON around it. Other tool
// inputs are small.
const maxBody = 12 << 20

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
	return guard(o, h)
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
	t.registerRCEP(s)
	t.registerUSMCA(s)
	t.registerEvidenceReport(s)
	t.registerAnswers(s)
	if o.MediaExtract {
		t.registerMedia(s)
	}
	if o.PartsHTS {
		t.registerPartsHTS(s)
	}
	if o.CanadaMarking {
		t.registerCanada(s)
	}
	if o.EUOrigin {
		t.registerOrigin(s, euOriginTool)
	}
	if o.UKOrigin {
		t.registerOrigin(s, ukOriginTool)
	}
	if o.ATIGA {
		t.registerATIGA(s)
	}
	if o.ADCVD {
		t.registerADCVD(s)
	}
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
// visitor ID are dropped (and a broker review token): an MCP caller is an
// API key or anonymous. The client address is resolved once with the same
// trusted-proxy rule as REST (#479): a caller-supplied X-Client-IP is used
// only when Options.ClientAddress says the connection is a trusted proxy,
// and the resolved value replaces the header before any tool sees it.
func guard(o Options, next http.Handler) http.Handler {
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
		for _, h := range []string{"Authorization", "X-API-Key", "X-User-Token", "X-Visitor-ID", "X-Review-Token", "X-Declaration-Token", "Cookie"} {
			r.Header.Del(h)
		}
		if key != "" {
			r.Header.Set("X-API-Key", key)
		}
		addr := ""
		if o.ClientAddress != nil {
			addr = strings.TrimSpace(o.ClientAddress(r.RemoteAddr, r.Header.Get("X-Client-IP")))
		}
		if addr == "" {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			addr = host
		}
		r.Header.Set("X-Client-IP", addr)
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
	raw          []byte // sent as is with contentType, instead of body
	contentType  string
	access       string // case access key (anonymous callers): sent as X-Visitor-ID
	idempotency  string // sent as Idempotency-Key (plan searches, D402)
}

// apiResult is the REST answer: the status and the decoded JSON body.
type apiResult struct {
	status int
	body   map[string]any
}

func (r apiResult) ok() bool { return r.status >= 200 && r.status < 300 }

// call dispatches c to the API handler with the caller's identity headers
// only (from is the MCP request's header, already mapped by guard).
//
// The request is built on isolated(ctx): the tool call is cancelled with the
// MCP request, but carries none of its context values. The API reads a
// verified sign-in, an API key and the audit state from the context, and its
// middleware has already run on POST /mcp, so a request built on that
// context would act as whoever the outer request was (#385). The tool call's
// principal comes only from the headers set here, which the API's middleware
// checks again on the way in.
func (t tools) call(ctx context.Context, from http.Header, c apiCall) (apiResult, error) {
	if !strings.HasPrefix(c.path, "/v1/") {
		return apiResult{}, fmt.Errorf("mcpserver: %q is not an API path", c.path)
	}
	var body io.Reader = http.NoBody
	if c.raw != nil {
		body = bytes.NewReader(c.raw)
	} else if c.body != nil {
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
	req, err := http.NewRequestWithContext(isolated(ctx), c.method, u, body)
	if err != nil {
		return apiResult{}, err
	}
	req.RequestURI = ""
	req.Host = "mcp.local"
	req.Header.Set("Accept", "application/json")
	if c.raw != nil {
		req.Header.Set("Content-Type", c.contentType)
	} else if c.body != nil {
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
	if c.idempotency != "" {
		req.Header.Set("Idempotency-Key", c.idempotency)
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

// valueless is a context's cancellation and deadline without its values.
type valueless struct{ context.Context }

func (valueless) Value(any) any { return nil }

// isolated returns ctx without any of its values (see call).
func isolated(ctx context.Context) context.Context { return valueless{ctx} }

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
