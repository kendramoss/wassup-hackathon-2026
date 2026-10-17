package llm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestFallbackOn429(t *testing.T) {
	primary := &Fake{Respond: func(Request) (Response, error) {
		return Response{}, &APIError{Provider: "vertex", StatusCode: 429, Body: "Resource exhausted"}
	}}
	fallback := FakeText(`{"origin_country":"Thailand","confidence":0.8}`)
	m := WithFallback(primary, fallback, "tuned-flash")
	resp, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon", Messages: []Message{{Role: RoleUser, Content: "facts"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ModelID != "tuned-flash" {
		t.Errorf("model id = %q", resp.ModelID)
	}
	if len(primary.Calls()) != 1 || len(fallback.Calls()) != 1 || fallback.Calls()[0].Model != "tuned-flash" {
		t.Errorf("calls primary %d fallback %#v", len(primary.Calls()), fallback.Calls())
	}
	if fallback.Calls()[0].Messages[0].Content != "facts" {
		t.Errorf("fallback request changed the message: %+v", fallback.Calls()[0])
	}
}

func TestFallbackOnDeadline(t *testing.T) {
	primary := &Fake{Respond: func(Request) (Response, error) {
		return Response{}, fmt.Errorf("vertex: %w", context.DeadlineExceeded)
	}}
	fallback := FakeText("ok")
	m := WithFallback(primary, fallback, "tuned-flash")
	resp, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon"})
	if err != nil || resp.ModelID != "tuned-flash" || len(fallback.Calls()) != 1 {
		t.Fatalf("resp %+v err %v calls %d", resp, err, len(fallback.Calls()))
	}
}

func TestFallbackOn504AfterRetries(t *testing.T) {
	primary := &Fake{Respond: func(Request) (Response, error) {
		return Response{}, &APIError{Provider: "vertex", StatusCode: 504, Body: "Deadline expired before operation could complete."}
	}}
	// The engine's primary is already retried; the wrapper sees the final error.
	wrapped := WithRetry(primary, 1, time.Millisecond)
	fallback := FakeText("ok")
	m := WithFallback(wrapped, fallback, "tuned-flash")
	resp, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon"})
	if err != nil || resp.ModelID != "tuned-flash" {
		t.Fatalf("resp %+v err %v", resp, err)
	}
	if len(primary.Calls()) != 2 || len(fallback.Calls()) != 1 {
		t.Errorf("primary attempts %d, fallback %d", len(primary.Calls()), len(fallback.Calls()))
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "client timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestFallbackOnClientTimeout(t *testing.T) {
	primary := &Fake{Respond: func(Request) (Response, error) {
		return Response{}, fmt.Errorf("vertex: %w", timeoutErr{})
	}}
	fallback := FakeText("ok")
	resp, err := WithFallback(primary, fallback, "tuned-flash").Generate(context.Background(), Request{Model: "gemini-4-argon"})
	if err != nil || resp.ModelID != "tuned-flash" || len(fallback.Calls()) != 1 {
		t.Fatalf("resp %+v err %v calls %d", resp, err, len(fallback.Calls()))
	}
}

func TestNoFallbackOn400OrBadResponse(t *testing.T) {
	for _, errIn := range []error{
		&APIError{Provider: "vertex", StatusCode: 400, Body: "bad request"},
		fmt.Errorf("vertex: %w: no JSON", ErrBadResponse),
		&APIError{Provider: "vertex", StatusCode: 500, Body: "internal"},
	} {
		primary := &Fake{Respond: func(Request) (Response, error) { return Response{}, errIn }}
		fallback := FakeText("should not run")
		_, err := WithFallback(primary, fallback, "tuned-flash").Generate(context.Background(), Request{Model: "gemini-4-argon"})
		if err == nil || len(fallback.Calls()) != 0 {
			t.Errorf("fell back on %v (err %v, calls %d)", errIn, err, len(fallback.Calls()))
		}
	}
}

func TestFallbackOffByDefault(t *testing.T) {
	primary := &Fake{Respond: func(Request) (Response, error) {
		return Response{}, &APIError{Provider: "vertex", StatusCode: 429, Body: "exhausted"}
	}}
	fallback := FakeText("no")
	m := WithFallback(primary, fallback, "")
	_, err := m.Generate(context.Background(), Request{Model: "gemini-4-argon"})
	if err == nil || len(fallback.Calls()) != 0 {
		t.Fatalf("empty fallback model still called fallback: %v calls %d", err, len(fallback.Calls()))
	}
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 429 {
		t.Errorf("err = %v", err)
	}
}
