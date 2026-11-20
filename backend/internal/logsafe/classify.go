package logsafe

import (
	"context"
	"errors"
	"log/slog"
	"net"
)

// Class is a member of a CLOSED set describing what kind of failure happened.
// It never carries text from the failure.
type Class string

const (
	ClassDeadline         Class = "deadline"
	ClassCanceled         Class = "canceled"
	ClassTimeout          Class = "timeout"
	ClassModelBadResponse Class = "model_bad_response"
	ClassModelUnavailable Class = "model_unavailable"
	ClassProviderHTTP4xx  Class = "provider_http_4xx"
	ClassProviderHTTP429  Class = "provider_http_429"
	ClassProviderHTTP5xx  Class = "provider_http_5xx"
	ClassStoreNotFound    Class = "store_not_found"
	ClassStoreOther       Class = "store_other"
	ClassInvalidInput     Class = "invalid_input"
	ClassAuthRefused      Class = "auth_refused"
	ClassParseFailure     Class = "parse_failure"
	ClassPanic            Class = "panic"
	ClassStdLog           Class = "stdlog"
	ClassUnclassified     Class = "unclassified"
)

var allClasses = []Class{
	ClassDeadline, ClassCanceled, ClassTimeout, ClassModelBadResponse, ClassModelUnavailable,
	ClassProviderHTTP4xx, ClassProviderHTTP429, ClassProviderHTTP5xx, ClassStoreNotFound,
	ClassStoreOther, ClassInvalidInput, ClassAuthRefused, ClassParseFailure, ClassPanic,
	ClassStdLog, ClassUnclassified,
}

// AllClasses returns the closed set.
func AllClasses() []Class { return append([]Class(nil), allClasses...) }

// Valid reports whether c is a member of the closed set.
func (c Class) Valid() bool {
	for _, k := range allClasses {
		if c == k {
			return true
		}
	}
	return false
}

func (c Class) String() string { return string(c) }

type classer interface{ LogClass() string }
type statuser interface{ LogStatus() int }

// Classify reduces err to a closed class and an optional HTTP status (0 when
// unknown). It uses errors.Is/As only and never calls Error(). An error may
// describe itself by implementing LogClass() string (validated against the
// closed set) and LogStatus() int (kept only in 100..599). Anything else is
// ClassUnclassified. A panic in any method of the error chain also gives
// ClassUnclassified.
func Classify(err error) (c Class, status int) {
	if err == nil {
		return "", 0
	}
	defer func() {
		if recover() != nil {
			c, status = ClassUnclassified, 0
		}
	}()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ClassDeadline, 0
	case errors.Is(err, context.Canceled):
		return ClassCanceled, 0
	}
	var cl classer
	if errors.As(err, &cl) {
		if k := Class(cl.LogClass()); k.Valid() {
			c = k
		}
	}
	if c == "" {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return ClassTimeout, 0
		}
		return ClassUnclassified, 0
	}
	var st statuser
	if errors.As(err, &st) {
		if s := st.LogStatus(); s >= 100 && s <= 599 {
			status = s
		}
	}
	return c, status
}

// errValue is logsafe's own, trusted error value (the only LogValuer the
// handler will resolve).
type errValue struct{ err error }

// Err wraps err for logging: only its class (and status) is emitted.
func Err(err error) slog.LogValuer { return errValue{err} }

func (e errValue) LogValue() slog.Value { return errGroup(e.err) }

func errGroup(err error) slog.Value {
	c, st := Classify(err)
	if c == "" {
		c = ClassUnclassified
	}
	attrs := []slog.Attr{slog.String("class", string(c))}
	if st > 0 {
		attrs = append(attrs, slog.Int("http_status", st))
	}
	return slog.GroupValue(attrs...)
}
