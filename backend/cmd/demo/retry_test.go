package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetriesWhenTheModelIsUnavailable(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bad":
			w.WriteHeader(http.StatusBadRequest)
		case n.Add(1) < 3:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"code":"model_unavailable"}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()
	c := client{base: srv.URL, http: srv.Client(), waits: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}}
	if b, err := c.callRaw("POST", "/plan", nil); err != nil || string(b) != `{"ok":true}` || n.Load() != 3 {
		t.Errorf("after two 502s: %s %v, %d calls", b, err, n.Load())
	}
	if _, err := c.callRaw("POST", "/bad", nil); err == nil {
		t.Error("a 400 must fail, not retry")
	}
	n.Store(-10)
	c.waits = c.waits[:1]
	if _, err := c.callRaw("POST", "/plan", nil); err == nil {
		t.Error("gave up too late or never")
	}
}
