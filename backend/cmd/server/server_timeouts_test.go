package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The server bounds reading a request and idle connections, but has no
// write deadline: a stream may outlast the read timeout (D137).
func TestServerTimeouts(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler(), nil)
	if srv.ReadHeaderTimeout != readHeaderTimeout || srv.ReadTimeout != readTimeout || srv.IdleTimeout != idleTimeout || srv.MaxHeaderBytes != maxHeaderBytes {
		t.Errorf("server = header %v read %v idle %v max header %d", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout, srv.MaxHeaderBytes)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v: it would cut off server-sent event streams", srv.WriteTimeout)
	}
}

// A stream that runs past ReadTimeout keeps its request context and
// reaches the client whole, for a GET and for a POST with a body.
func TestStreamOutlastsReadTimeout(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 4 {
			time.Sleep(150 * time.Millisecond)
			if err := r.Context().Err(); err != nil {
				fmt.Fprintf(w, "event: error\ndata: %v\n\n", err)
				return
			}
			fmt.Fprintf(w, "event: stage\ndata: %d\n\n", i)
			_ = http.NewResponseController(w).Flush()
		}
		fmt.Fprint(w, "event: done\ndata: {}\n\n")
	})
	srv := newServer("", h, nil)
	srv.ReadTimeout = 200 * time.Millisecond // the stream takes about 600 ms
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	url := "http://" + ln.Addr().String()
	for _, method := range []string{"GET", "POST"} {
		req, _ := http.NewRequest(method, url, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || !strings.HasSuffix(string(b), "event: done\ndata: {}\n\n") || strings.Contains(string(b), "event: error") {
			t.Errorf("%s stream = %q %v", method, b, err)
		}
	}
}
