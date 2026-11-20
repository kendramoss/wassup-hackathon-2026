package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/httpapi"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Synthetic sentinels (#285). Nothing here is a real name, address or secret.
const (
	sentEmail    = "zz.user285+leak@example.invalid"
	sentToken    = "tok_285_SYNTHETIC_DO_NOT_USE_0123456789"
	sentKey      = "op_285SYNTHETICKEY000000000000"
	sentSupplier = "Zzyzx-Synthetic-Supplier-285 GmbH"
	sentBOM      = "PN-285-SYNTH-0001 stator laminations 4200 pcs @ USD 1.234"
	sentAdmin    = "admin-285-synthetic-token-0123456789abcdef"
	sentActorKey = "audit-285-synthetic-actor-key-0123456789abcdef"
)

var sentinels = []string{sentEmail, sentToken, sentKey, sentSupplier, sentBOM, sentAdmin, sentActorKey, "Zzyzx", "PN-285-SYNTH", "zz.user285"}

func noSentinel(t *testing.T, where, out string) {
	t.Helper()
	for _, s := range sentinels {
		if strings.Contains(out, s) {
			t.Errorf("%s leaked %q:\n%s", where, s, out)
		}
	}
}

func envOf(extra map[string]string) func(string) string {
	m := map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CASES_STORE": "memory", "LLM_PROVIDER": "fake"}
	for k, v := range extra {
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func sentinelConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.FromEnv(envOf(map[string]string{"ADMIN_TOKEN": sentAdmin, "AUDIT_ACTOR_KEY": sentActorKey, "AUDIT": "on"}))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// The config summary logged at startup holds no secret, and the policy accepts
// that exact string only: a secret field logged by mistake is masked.
func TestStartupConfigStringDoesNotLeakSecrets(t *testing.T) {
	cfg := sentinelConfig(t)
	if cfg.AdminToken != sentAdmin || cfg.AuditActorKey != sentActorKey {
		t.Fatal("the sentinel secrets were not loaded")
	}
	noSentinel(t, "cfg.String()", cfg.String())
	var buf syncBuf
	logger, _, err := newLogger(&buf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("config", "summary", cfg.String())
	if !strings.Contains(buf.String(), `"summary":"provider=`) {
		t.Errorf("the config summary was not logged:\n%s", buf.String())
	}
	logger.Info("config", "summary", cfg.String()+" admin="+cfg.AdminToken, "admin_token", cfg.AdminToken, "audit_actor_key", cfg.AuditActorKey)
	noSentinel(t, "root logger", buf.String())
}

// The root logger is the allowlist: unknown keys, groups and messages, a
// token-shaped request id, error text and Stringers never reach the output.
func TestRootLoggerEntrypointBlocksSentinels(t *testing.T) {
	var buf syncBuf
	cfg := sentinelConfig(t)
	logger, _, err := newLogger(&buf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info(sentSupplier, "supplier", sentSupplier, sentKey, "x", "request_id", sentToken, "model", sentToken, "route", "GET /v1/cases/"+sentEmail)
	logger.Error("fatal", "err", errors.New(sentBOM+sentEmail), "detail", &llm.APIError{Body: sentBOM})
	logger.Warn("llm call failed", "err", logsafe.Err(&llm.APIError{StatusCode: 503, Body: sentBOM}), "tool_calls", []string{sentToken})
	logger.WithGroup(sentSupplier).Info("request", "status", 200)
	logger.With("token", sentToken).Info("request", "status", 200)
	noSentinel(t, "root logger", buf.String())
	for _, want := range []string{`"class":"provider_http_5xx"`, `"http_status":503`, `[message masked]`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %s:\n%s", want, buf.String())
		}
	}
	// slog.SetDefault-style users (log.Print) are bridged and constant.
	slog.New(logger.Handler()).Info("request", "status", 200)
}

// Run the real startup wiring (buildModels, newDeps with the four engines,
// newAudit, newCases) under the PRODUCTION policy: every startup line is kept
// whole, nothing operational is masked, and no sentinel appears. Audit is
// switched on here, off in the next test; the policy does not depend on it.
func TestStartupLinesAreKeptUnderTheProductionPolicy(t *testing.T) {
	for _, auditOn := range []bool{true, false} {
		name := "audit off"
		if auditOn {
			name = "audit on"
		}
		t.Run(name, func(t *testing.T) {
			var buf syncBuf
			var lh *logsafe.Handler
			engineEnvLogger = func(cfg config.Config) *slog.Logger {
				h, err := logsafe.NewHandler(slog.NewJSONHandler(&buf, nil), logPolicy(cfg))
				if err != nil {
					t.Fatal(err)
				}
				lh = h
				return slog.New(h)
			}
			t.Cleanup(func() { engineEnvLogger = nil })
			t.Setenv("ADMIN_TOKEN", sentAdmin)
			if auditOn {
				t.Setenv("AUDIT_ACTOR_KEY", sentActorKey)
				t.Setenv("AUDIT", "on")
			} else {
				t.Setenv("AUDIT", "off")
			}
			t.Setenv("HTS_SUGGEST", "off")
			t.Setenv("TEAMS", "off")
			engineEnv(t, "gemini-3.8-flash", &llm.Fake{Respond: func(llm.Request) (llm.Response, error) { return llm.Response{Text: "{}"}, nil }}, nil)
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			logger := slog.New(lh)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if _, err := newCases(ctx, cfg, logger); err != nil {
				t.Fatal(err)
			}
			if _, err := newAudit(ctx, cfg, store.NewMemory(), logger); err != nil {
				t.Fatal(err)
			}
			logger.Info("config", "summary", cfg.String())
			logger.Info("listening", "port", cfg.Port, "llm_provider", "fake")
			out := buf.String()
			noSentinel(t, name, out)
			if st := lh.Stats(); st != (logsafe.Stats{}) {
				t.Errorf("the policy masked or dropped startup output: %+v\n%s", st, out)
			}
			for _, want := range []string{`"msg":"corpus loaded"`, `"msg":"engine fallback on"`, `"msg":"textile engine on"`, `"msg":"config"`, `"msg":"listening"`} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %s:\n%s", want, out)
				}
			}
			wantAudit := `"msg":"audit trail NOT enabled`
			if auditOn {
				wantAudit = `"msg":"audit trail on"`
			}
			if !strings.Contains(out, wantAudit) {
				t.Errorf("missing %s:\n%s", wantAudit, out)
			}
		})
	}
}

func serverWith(t *testing.T, h http.Handler, buf *syncBuf, cfg config.Config) *httptest.Server {
	t.Helper()
	_, el, err := newLogger(buf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = el
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// net/http's own error log goes through the constant bridge. The bridge never
// recovers: http.ErrAbortHandler still aborts the connection quietly, a panic
// still closes the connection (no blind 500), and bytes already streamed to a
// client stay delivered with no second body appended.
func TestErrorLogBridgeKeepsNetHTTPPanicSemantics(t *testing.T) {
	cfg := sentinelConfig(t)
	var buf syncBuf
	mux := http.NewServeMux()
	mux.HandleFunc("/panic", func(http.ResponseWriter, *http.Request) { panic(sentBOM + " " + sentEmail) })
	mux.HandleFunc("/abort", func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "event: progress\ndata: {\"n\":1}\n\n")
		w.(http.Flusher).Flush()
		panic(sentSupplier)
	})
	srv := serverWith(t, mux, &buf, cfg)
	c := &http.Client{Timeout: 5 * time.Second}

	// A handler panic: no response (net/http closes the connection), not a 500.
	if resp, err := c.Get(srv.URL + "/panic"); err == nil {
		t.Errorf("panic: got a response %d %v, want the connection closed (no blind 500)", resp.StatusCode, resp.Header)
		resp.Body.Close()
	}
	// ErrAbortHandler: aborted, and net/http logs nothing for it.
	before := strings.Count(buf.String(), "http server log line")
	if resp, err := c.Get(srv.URL + "/abort"); err == nil {
		t.Errorf("abort: got a response %d", resp.StatusCode)
		resp.Body.Close()
	}
	if got := strings.Count(buf.String(), "http server log line"); got != before {
		t.Errorf("ErrAbortHandler produced %d log line(s), want none", got-before)
	}
	// A committed SSE stream: the status and the event already sent stay as they were.
	resp, err := c.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	body, rerr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `event: progress`) || strings.Contains(string(body), `"code"`) || strings.Contains(string(body), "internal") {
		t.Errorf("sse: status %d body %q (read error %v): the committed stream was changed", resp.StatusCode, body, rerr)
	}
	time.Sleep(50 * time.Millisecond)
	out := buf.String()
	noSentinel(t, "ErrorLog", out)
	if !strings.Contains(out, `"msg":"http server log line"`) || !strings.Contains(out, `"class":"stdlog"`) {
		t.Errorf("the panic was not reported as the constant stdlog record:\n%s", out)
	}
}

// The full HTTP stack as run() assembles it: the real handler behind the root
// logger and ErrorLog over a real listener, sentinels in the path, query and
// headers. (Audit on and off, with the real audit middleware and store, is
// covered in internal/httpapi: the log policy reads no audit state.)
func TestServerStackNeverLogsRequestText(t *testing.T) {
	var buf syncBuf
	cfg := sentinelConfig(t)
	logger, el, err := newLogger(&buf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(httpapi.NewHandler(logger, httpapi.Info{Provider: "fake"}))
	srv.Config.ErrorLog = el
	srv.Start()
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/v1/cases/"+sentSupplier+"?token="+sentToken, nil)
	req.Header.Set("X-Request-ID", sentToken)
	req.Header.Set("X-User-Token", sentToken)
	req.Header.Set("X-API-Key", sentKey)
	req.Header.Set("X-Visitor-ID", sentEmail)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); !strings.HasPrefix(got, "h_") || got == sentToken {
		t.Errorf("X-Request-ID = %q", got)
	}
	out := buf.String()
	noSentinel(t, "server stack", out)
	if !strings.Contains(out, `"request_id":"`+resp.Header.Get("X-Request-ID")+`"`) || !strings.Contains(out, `"route":"/"`) {
		t.Errorf("operational fields missing or masked:\n%s", out)
	}
}

// One constructor, complete policy: the root handler is a single logsafe
// handler whose closed sets come from the real route table, the configured
// models and the registered tools. A second wrap with any other policy is
// refused (never silently ignored), and nothing is added after construction.
func TestRootLoggerPolicyIsCompleteAndWrappedOnce(t *testing.T) {
	cfg := sentinelConfig(t)
	var buf syncBuf
	logger, _, err := newLogger(&buf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := logger.Handler().(*logsafe.Handler)
	if !ok {
		t.Fatalf("the root handler is %T, want *logsafe.Handler", logger.Handler())
	}
	if _, err := logsafe.NewHandler(h, logPolicy(cfg)); !errors.Is(err, logsafe.ErrAlreadyWrapped) {
		t.Errorf("wrapping the root handler again with another policy: %v, want ErrAlreadyWrapped", err)
	}
	for _, p := range httpapi.RoutePatterns() {
		logger.Info("request", "route", p, "status", 200)
	}
	logger.Info("llm call", "model", cfg.EngineModel, "model_id", cfg.EngineModel+"-001", "tool_calls", httpapi.LogToolNames, "finish_reason", "STOP")
	if st := h.Stats(); st != (logsafe.Stats{}) {
		t.Fatalf("a registered route, model or tool was masked: %+v\n%s", st, buf.String())
	}
	buf.Reset()
	logger.Info("llm call", "model", sentToken, "model_id", "gemini-unconfigured-9", "tool_calls", []string{"pn_285_synth_0001"})
	logger.Info("request", "route", "GET /v1/cases/"+sentSupplier)
	noSentinel(t, "unregistered values", buf.String())
	if strings.Contains(buf.String(), "gemini-unconfigured-9") || strings.Contains(buf.String(), "pn_285_synth_0001") {
		t.Errorf("a value outside the closed sets was logged:\n%s", buf.String())
	}
}

// syncBuf is a bytes.Buffer safe for the server goroutines that log into it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
func (s *syncBuf) Reset()                      { s.mu.Lock(); defer s.mu.Unlock(); s.b.Reset() }
