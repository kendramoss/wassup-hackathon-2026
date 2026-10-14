package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
