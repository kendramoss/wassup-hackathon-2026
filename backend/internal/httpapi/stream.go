package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// keepAlive is how often an idle event stream sends an SSE comment, so
// proxies and Cloud Run keep the connection open.
var keepAlive = 10 * time.Second

// wantsStream reports whether the client asked for server-sent events.
func wantsStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// eventStream writes server-sent events, with a ": keep-alive" comment every
// keepAlive. close must run before the handler returns: it stops the
// keep-alive writer and waits for it, because a ResponseWriter may not be
// used after the handler returns (it raced with the test recorder's reads,
// and on a real server it could write to a reused connection; D55). Sends
// after close are dropped.
type eventStream struct {
	mu            sync.Mutex
	w             http.ResponseWriter
	rc            *http.ResponseController
	closed        bool
	stop, stopped chan struct{}
}

// openStream sends the SSE headers and starts the keep-alive writer.
func openStream(w http.ResponseWriter) *eventStream {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // the work outlives the server's default write timeout
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s := &eventStream{w: w, rc: rc, stop: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(s.stopped)
		t := time.NewTicker(keepAlive)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.write(": keep-alive\n\n")
			case <-s.stop:
				return
			}
		}
	}()
	return s
}

func (s *eventStream) write(chunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	_, _ = io.WriteString(s.w, chunk)
	_ = s.rc.Flush()
}

// send writes one event with v as its JSON data.
func (s *eventStream) send(event string, v any) {
	b, _ := json.Marshal(v)
	s.write("event: " + event + "\ndata: " + string(b) + "\n\n")
}

// close stops the keep-alive writer and waits for it to exit.
func (s *eventStream) close() {
	close(s.stop)
	<-s.stopped
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

// Stage statuses in the streamed predict's "stage" events (D71).
const (
	stageStarted = "started"
	stageDone    = "done"
)

// stages reports predict's steps (router, precedents, engine, clarify) as
// they start and finish. Its methods are safe from several goroutines (the
// router runs beside the engine) and do nothing on a nil *stages, so the
// JSON path passes nil. They only observe: results do not depend on them.
type stages struct {
	mu    sync.Mutex
	start time.Time
	began map[string]time.Time
	done  map[string]bool
	// emit gets ms since the request began for started, and the stage's
	// duration for done.
	emit func(name, status string, ms int64)
}

func newStages(start time.Time, emit func(name, status string, ms int64)) *stages {
	return &stages{start: start, began: map[string]time.Time{}, done: map[string]bool{}, emit: emit}
}

// begin reports a stage started, unless it is already running. A stage that
// finished may run again (the textile engine after the main one).
func (s *stages) begin(name string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beginLocked(name)
}

func (s *stages) beginLocked(name string) {
	if _, ok := s.began[name]; ok && !s.done[name] {
		return
	}
	now := time.Now()
	s.began[name], s.done[name] = now, false
	s.emit(name, stageStarted, now.Sub(s.start).Milliseconds())
}

// end reports a stage done, unless it already is. A stage that never
// reported its start (a cached prediction does no retrieval or model call)
// is reported started and done at once.
func (s *stages) end(name string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done[name] {
		return
	}
	if _, ok := s.began[name]; !ok {
		s.beginLocked(name)
	}
	s.done[name] = true
	s.emit(name, stageDone, time.Since(s.began[name]).Milliseconds())
}

// settle reports every running stage done (after a step that may have
// failed part-way but whose failure the analysis absorbs).
func (s *stages) settle() {
	if s == nil {
		return
	}
	s.mu.Lock()
	var running []string
	for name := range s.began {
		if !s.done[name] {
			running = append(running, name)
		}
	}
	s.mu.Unlock()
	sort.Strings(running)
	for _, name := range running {
		s.end(name)
	}
}

// engineProgress adapts s to the engine's progress hook (engine.Progress).
func (s *stages) engineProgress(stage string, done bool) {
	if done {
		s.end(stage)
	} else {
		s.begin(stage)
	}
}
