package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// suiteHandler is the production allowlist handler (LogPolicy) over a
// discarded JSON writer. Every harness that does not inject its own logger
// logs through it, so the whole suite exercises the real call sites under the
// real policy. TestMain then fails the run if the policy masked or dropped
// anything: an unregistered key, message, route, model, tool or value at any
// site that the suite reaches shows up here (#285).
var suiteHandler = func() *logsafe.Handler {
	h, err := logsafe.NewHandler(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
		LogPolicy("gemini-4-argon", "gemini-3.8-flash"))
	if err != nil {
		panic(err)
	}
	return h
}()

func TestMain(m *testing.M) {
	code := m.Run()
	if st := suiteHandler.Stats(); code == 0 && st != (logsafe.Stats{}) {
		fmt.Fprintf(os.Stderr, "FAIL: the production log policy masked or dropped output in the suite: %+v\n", st)
		code = 1
	}
	os.Exit(code)
}
