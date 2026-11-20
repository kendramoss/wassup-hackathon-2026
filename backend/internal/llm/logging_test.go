package llm

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// Synthetic sentinels only (#285). None is a real name, address or secret.
const (
	sentSupplier = "Zzyzx-Synthetic-Supplier-285 GmbH"
	sentBOM      = "PN-285-SYNTH-0001 stator laminations 4200 pcs @ USD 1.234"
	sentEmail    = "zz.user285+leak@example.invalid"
	sentToken    = "tok_285_SYNTHETIC_DO_NOT_USE_0123456789"
)

var allSentinels = []string{sentSupplier, sentBOM, sentEmail, sentToken, "Zzyzx", "PN-285-SYNTH"}

func assertNoSentinel(t *testing.T, where, out string) {
	t.Helper()
	for _, s := range allSentinels {
		if strings.Contains(out, s) {
			t.Errorf("%s leaked sentinel %q:\n%s", where, s, out)
		}
	}
}

// debugLoggers returns the two handlers the unit must be safe under: a plain
// JSON handler at DEBUG (what a developer or a misconfigured deploy would
// inject) and the production allowlist handler at DEBUG.
func debugLoggers(t *testing.T) map[string]*slog.Logger {
	t.Helper()
	mk := func(wrapped bool) (*slog.Logger, *bytes.Buffer) {
		var buf bytes.Buffer
		var h slog.Handler = slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		if wrapped {
			p := logsafe.DefaultPolicy().AllowModels("gemini-4-argon").AllowTools("get_case", "search_rulings")
			lh, err := logsafe.NewHandler(h, p)
			if err != nil {
				t.Fatal(err)
			}
			h = lh
		}
		return slog.New(h), &buf
	}
	plain, pb := mk(false)
	safe, sb := mk(true)
	bufs[plain], bufs[safe] = pb, sb
	return map[string]*slog.Logger{"plain": plain, "allowlist": safe}
}

var bufs = map[*slog.Logger]*bytes.Buffer{}

func TestLoggingNoContentAtDebug(t *testing.T) {
	for name, lg := range debugLoggers(t) {
		t.Run(name, func(t *testing.T) {
			m := WithLogging(&Fake{Respond: func(Request) (Response, error) {
				return Response{Text: "answer about " + sentBOM + " from " + sentSupplier,
					Usage:        Usage{InputTokens: 12, OutputTokens: 34, TotalTokens: 46},
					ToolCalls:    []ToolCall{{Name: "get_case"}},
					FinishReason: "STOP", ModelID: "gemini-4-argon"}, nil
			}}, lg)
			_, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon", System: "system with " + sentSupplier,
				Messages: []Message{{Role: RoleUser, Content: sentBOM + " " + sentEmail + " " + sentToken}}})
			if err != nil {
				t.Fatal(err)
			}
			out := bufs[lg].String()
			assertNoSentinel(t, name+" DEBUG", out)
			// Sizes only at DEBUG, and the INFO line keeps its operational fields.
			for _, want := range []string{`"msg":"llm call sizes"`, `"system_chars"`, `"text_chars"`, `"msg":"llm call"`,
				`"provider":"fake"`, `"model":"gemini-4-argon"`, `"input_tokens":12`, `"output_tokens":34`, `"latency_ms"`} {
				if !strings.Contains(out, want) {
					t.Errorf("%s: missing %s in:\n%s", name, want, out)
				}
			}
			if strings.Contains(out, "llm call content") {
				t.Errorf("%s: the raw content line still exists:\n%s", name, out)
			}
		})
	}
}

func TestLoggingOperationalFieldsKeptUnderAllowlist(t *testing.T) {
	lg := debugLoggers(t)["allowlist"]
	m := WithLogging(&Fake{Respond: func(Request) (Response, error) {
		return Response{Text: "x", Usage: Usage{InputTokens: 1, OutputTokens: 2}, ToolCalls: []ToolCall{{Name: "get_case"}, {Name: "search_rulings"}},
			FinishReason: "STOP", ModelID: "gemini-4-argon-001"}, nil
	}}, lg)
	if _, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon"}); err != nil {
		t.Fatal(err)
	}
	out := bufs[lg].String()
	for _, want := range []string{`"model_id":"gemini-4-argon-001"`, `"tool_calls":["get_case","search_rulings"]`, `"finish_reason":"STOP"`, `"model":"gemini-4-argon"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[masked]") || strings.Contains(out, "masked_attrs") {
		t.Errorf("an operational field was masked:\n%s", out)
	}
}

// A model-controlled tool name or finish reason that is not in the closed set
// is masked, not passed.
func TestLoggingTokenShapedProviderValuesAreMasked(t *testing.T) {
	lg := debugLoggers(t)["allowlist"]
	m := WithLogging(&Fake{Respond: func(Request) (Response, error) {
		return Response{Text: "x", ToolCalls: []ToolCall{{Name: "pn_285_synth_0001"}}, FinishReason: sentToken, ModelID: sentToken}, nil
	}}, lg)
	if _, err := m.Generate(context.Background(), Request{Model: sentToken}); err != nil {
		t.Fatal(err)
	}
	assertNoSentinel(t, "provider values", bufs[lg].String())
	if strings.Contains(bufs[lg].String(), "pn_285_synth_0001") {
		t.Errorf("an unregistered tool name was logged:\n%s", bufs[lg].String())
	}
}

func TestLoggingErrorClassOnly(t *testing.T) {
	cases := map[string]error{
		"api error":    &APIError{Provider: "vertex", StatusCode: 429, Body: sentBOM + " " + sentEmail},
		"api 500":      &APIError{Provider: "vertex", StatusCode: 503, Body: sentSupplier},
		"bad response": wrapText(ErrBadResponse, sentBOM),
		"unavailable":  wrapText(ErrUnavailable, sentEmail),
		"other":        wrapText(nil, sentToken),
	}
	for name, lg := range debugLoggers(t) {
		for cname, cerr := range cases {
			t.Run(name+"/"+cname, func(t *testing.T) {
				bufs[lg].Reset()
				e := cerr
				m := WithLogging(&Fake{Respond: func(Request) (Response, error) { return Response{Text: sentBOM}, e }}, lg)
				if _, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon", System: sentSupplier}); err == nil {
					t.Fatal("want an error")
				}
				out := bufs[lg].String()
				assertNoSentinel(t, name+"/"+cname, out)
				if !strings.Contains(out, `"class"`) {
					t.Errorf("no error class in:\n%s", out)
				}
			})
		}
	}
}

func TestErrorClasses(t *testing.T) {
	cases := []struct {
		err    error
		class  logsafe.Class
		status int
	}{
		{&APIError{StatusCode: 429, Body: sentBOM}, logsafe.ClassProviderHTTP429, 429},
		{&APIError{StatusCode: 400, Body: sentBOM}, logsafe.ClassProviderHTTP4xx, 400},
		{&APIError{StatusCode: 503, Body: sentBOM}, logsafe.ClassProviderHTTP5xx, 503},
		{wrapText(ErrBadResponse, sentBOM), logsafe.ClassModelBadResponse, 0},
		{wrapText(ErrUnavailable, sentBOM), logsafe.ClassModelUnavailable, 0},
	}
	for _, c := range cases {
		cl, st := logsafe.Classify(c.err)
		if cl != c.class || st != c.status {
			t.Errorf("%v: got %s/%d, want %s/%d", c.err, cl, st, c.class, c.status)
		}
	}
}

type textErr struct {
	text string
	err  error
}

func (e textErr) Error() string { return e.text }
func (e textErr) Unwrap() error { return e.err }

func wrapText(cause error, text string) error { return textErr{"vertex: " + text, cause} }
