package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestGetTime(t *testing.T) {
	got, err := getTime(nil, getTimeArgs{Timezone: "Asia/Singapore"})
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, got.Time)
	if err != nil {
		t.Fatalf("time %q is not RFC 3339: %v", got.Time, err)
	}
	if _, off := ts.Zone(); off != 8*60*60 {
		t.Errorf("offset = %ds, want +8h", off)
	}

	if got, _ := getTime(nil, getTimeArgs{}); got.Timezone != "UTC" {
		t.Errorf("default timezone = %q, want UTC", got.Timezone)
	}
	if _, err := getTime(nil, getTimeArgs{Timezone: "Mars/Olympus"}); err == nil {
		t.Error("unknown timezone: want error")
	}
}

// These requests are all rejected before the agent runs, so no runner is needed.
func TestChatHandlerRejectsBadInput(t *testing.T) {
	h := chatHandler(nil)
	tests := []struct {
		name, method, body string
		want               int
	}{
		{"GET", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"not JSON", http.MethodPost, "nope", http.StatusBadRequest},
		{"empty message", http.MethodPost, `{"message":"  "}`, http.StatusBadRequest},
		{"long message", http.MethodPost, `{"message":"` + strings.Repeat("a", maxMessageRunes+1) + `"}`, http.StatusRequestEntityTooLarge},
		{"huge body", http.MethodPost, `{"message":"` + strings.Repeat("a", maxBodyBytes) + `"}`, http.StatusBadRequest},
		{"bad session id", http.MethodPost, `{"message":"hi","session_id":"../../etc"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h(w, httptest.NewRequest(tt.method, "/chat", strings.NewReader(tt.body)))
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d (%s)", w.Code, tt.want, strings.TrimSpace(w.Body.String()))
			}
		})
	}
}

func TestChatStreamHandlerRejectsBadInput(t *testing.T) {
	w := httptest.NewRecorder()
	chatStreamHandler(nil)(w, httptest.NewRequest(http.MethodPost, "/chat/stream", strings.NewReader(`{"message":""}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func textEvent(text string, partial bool) *session.Event {
	ev := &session.Event{}
	ev.Content = genai.NewContentFromText(text, genai.RoleModel)
	ev.Partial = partial
	return ev
}

func TestEventTranslator(t *testing.T) {
	call := &session.Event{}
	call.Content = genai.NewContentFromFunctionCall("get_time", map[string]any{"timezone": "Asia/Tokyo"}, genai.RoleModel)
	call.Content.Parts[0].FunctionCall.ID = "c1"
	result := &session.Event{}
	result.Content = genai.NewContentFromFunctionResponse("get_time", map[string]any{"time": "now"}, genai.RoleUser)
	result.Content.Parts[0].FunctionResponse.ID = "c1"
	thought := textEvent("thinking...", false)
	thought.Content.Parts[0].Thought = true

	partialCall := &session.Event{}
	partialCall.Content = call.Content
	partialCall.Partial = true

	events := []*session.Event{
		partialCall, call, // streamed call, then its repeat: call kept once
		result,
		textEvent("It's ", true), textEvent("noon.", true),
		textEvent("It's noon.", false), // streaming repeat of the full text: dropped
		thought,                        // model thoughts: dropped
		textEvent("Bye.", false),       // non-streaming text: kept
	}
	var tr eventTranslator
	var got []string
	for _, ev := range events {
		for _, e := range tr.translate(ev) {
			got = append(got, e.Type+":"+e.Name+e.Text+e.ID)
		}
	}
	want := []string{"tool_call:get_timec1", "tool_result:get_timec1", "text:It's ", "text:noon.", "text:Bye."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestDropEmptyParts(t *testing.T) {
	sigOnly := &genai.Part{ThoughtSignature: []byte("sig")}
	text := genai.NewPartFromText("hi")
	text.ThoughtSignature = []byte("sig")
	stored := &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{text, sigOnly}}
	contents := []*genai.Content{
		genai.NewContentFromText("q", genai.RoleUser),
		stored,
		{Role: genai.RoleModel, Parts: []*genai.Part{sigOnly}}, // nothing left: dropped
	}

	got := dropEmptyParts(contents)
	if len(got) != 2 || len(got[1].Parts) != 1 || got[1].Parts[0].Text != "hi" {
		t.Fatalf("got %d contents, want 2 with the signature-only parts removed", len(got))
	}
	if len(stored.Parts) != 2 {
		t.Error("input content was modified")
	}
}
