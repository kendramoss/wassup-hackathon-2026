package logsafe

import (
	"context"
	"log"
	"log/slog"
	"time"
)

type stdBridge struct{ h slog.Handler }

// Write discards the bytes of the original line (they may hold a panic value,
// a remote address or request text) and emits one constant record.
func (b stdBridge) Write(p []byte) (int, error) {
	n := len(p)
	defer func() { _ = recover() }()
	r := slog.NewRecord(time.Now(), slog.LevelError, msgStdLog, 0)
	r.AddAttrs(slog.String("class", string(ClassStdLog)))
	_ = b.h.Handle(context.Background(), r)
	return n, nil
}

// ErrorLog returns a *log.Logger for http.Server.ErrorLog (and any other
// std-log sink) that reports each line as a constant record of class stdlog.
// The original text is never printed.
func ErrorLog(h slog.Handler) *log.Logger { return log.New(stdBridge{h}, "", 0) }
