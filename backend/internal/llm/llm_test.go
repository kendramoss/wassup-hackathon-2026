package llm

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestDecodeJSON(t *testing.T) {
	var v struct {
		Origin string `json:"origin_country"`
	}
	for _, text := range []string{
		`{"origin_country":"China"}`,
		"```json\n{\"origin_country\":\"China\"}\n```",
		`Here you go: {"origin_country": "China"} done`,
	} {
		v.Origin = ""
		if err := (Response{Text: text}).DecodeJSON(&v); err != nil || v.Origin != "China" {
			t.Errorf("DecodeJSON(%q) = %v, origin %q", text, err, v.Origin)
		}
	}
	for _, text := range []string{"", "no json", "{broken"} {
		if err := (Response{Text: text}).DecodeJSON(&v); !errors.Is(err, ErrBadResponse) {
			t.Errorf("DecodeJSON(%q) error = %v, want ErrBadResponse", text, err)
		}
	}
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name      string
		errs      []error // returned in order; then success
		wantCalls int
		wantErr   error
	}{
		{"success first time", nil, 1, nil},
		{"429 then ok", []error{&APIError{StatusCode: 429}}, 2, nil},
		{"503 twice then ok", []error{&APIError{StatusCode: 503}, &APIError{StatusCode: 503}}, 3, nil},
		{"400 is not retried", []error{&APIError{StatusCode: 400}}, 1, &APIError{}},
		{"bad response is not retried", []error{ErrBadResponse}, 1, ErrBadResponse},
		{"gives up after max retries", []error{&APIError{StatusCode: 500}, &APIError{StatusCode: 500}, &APIError{StatusCode: 500}, &APIError{StatusCode: 500}}, 4, ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := 0
			f := &Fake{Respond: func(Request) (Response, error) {
				n++
				if n <= len(tt.errs) {
					return Response{}, tt.errs[n-1]
				}
				return Response{Text: "ok"}, nil
			}}
			_, err := WithRetry(f, 3, time.Millisecond).Generate(context.Background(), Request{})
			if n != tt.wantCalls {
				t.Errorf("calls = %d, want %d", n, tt.wantCalls)
			}
			var apiErr *APIError
			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("unexpected error %v", err)
			case errors.As(tt.wantErr, &apiErr) && !errors.As(err, &apiErr):
				t.Errorf("error %v, want APIError", err)
			case tt.wantErr != nil && !errors.As(tt.wantErr, &apiErr) && !errors.Is(err, tt.wantErr):
				t.Errorf("error %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRetryStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &Fake{Respond: func(Request) (Response, error) { cancel(); return Response{}, &APIError{StatusCode: 503} }}
	if _, err := WithRetry(f, 3, time.Hour).Generate(ctx, Request{}); err == nil {
		t.Fatal("want error after cancel")
	}
	if len(f.Calls()) != 1 {
		t.Fatalf("calls = %d, want 1", len(f.Calls()))
	}
}

func TestLoggingOmitsContentAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	f := &Fake{Respond: func(Request) (Response, error) {
		return Response{Text: "SECRET-ANSWER", Usage: Usage{InputTokens: 10, OutputTokens: 5},
			ToolCalls: []ToolCall{{Name: "search_rulings"}}}, nil
	}}
	_, err := WithLogging(f, logger).Generate(context.Background(), Request{Model: "m", System: "SECRET-BOM"})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`"input_tokens":10`, `"output_tokens":5`, `"search_rulings"`, `"provider":"fake"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %s: %s", want, out)
		}
	}
	if strings.Contains(out, "SECRET") {
		t.Errorf("info log contains prompt or answer: %s", out)
	}
}
