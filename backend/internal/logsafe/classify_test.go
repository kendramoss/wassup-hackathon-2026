package logsafe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func timeZero() time.Time { return time.Time{} }

type metaErr struct {
	class  string
	status int
	called *bool
}

func (e metaErr) Error() string {
	if e.called != nil {
		*e.called = true
	}
	return "provider said: " + sentBOM + " " + sentEmail
}
func (e metaErr) LogClass() string { return e.class }
func (e metaErr) LogStatus() int   { return e.status }

type panicIs struct{}

func (panicIs) Error() string    { return sentSupplier }
func (panicIs) Is(error) bool    { panic(sentSupplier) }
func (panicIs) Unwrap() error    { panic(sentSupplier) }
func (panicIs) LogClass() string { panic(sentSupplier) }

func TestClassify(t *testing.T) {
	var called bool
	cases := []struct {
		name   string
		err    error
		class  Class
		status int
	}{
		{"nil", nil, "", 0},
		{"deadline", fmt.Errorf("wrap %s: %w", sentEmail, context.DeadlineExceeded), ClassDeadline, 0},
		{"canceled", fmt.Errorf("x: %w", context.Canceled), ClassCanceled, 0},
		{"metadata", fmt.Errorf("ctx %s: %w", sentSupplier, metaErr{ClassModelUnavailable.String(), 503, &called}), ClassModelUnavailable, 503},
		{"joined", errors.Join(errors.New(sentBOM), metaErr{ClassModelBadResponse.String(), 0, &called}), ClassModelBadResponse, 0},
		{"unknown class value", metaErr{"wild_" + sentSupplier, 0, &called}, ClassUnclassified, 0},
		{"status out of range", metaErr{ClassStoreOther.String(), 99999, &called}, ClassStoreOther, 0},
		{"plain unknown", errors.New(sentToken), ClassUnclassified, 0},
		{"panicking methods", fmt.Errorf("a: %w", panicIs{}), ClassUnclassified, 0},
	}
	called = false // constructing the wrapped errors above formatted them with %w
	for _, c := range cases {
		got, st := Classify(c.err)
		if got != c.class || st != c.status {
			t.Errorf("%s: got %q/%d want %q/%d", c.name, got, st, c.class, c.status)
		}
	}
	if called {
		t.Fatal("Error() of an error was called while classifying")
	}
}

func TestClassSetIsClosed(t *testing.T) {
	for _, c := range AllClasses() {
		if !c.Valid() {
			t.Errorf("%q not valid", c)
		}
		if strings.ContainsAny(c.String(), " @:") {
			t.Errorf("class %q is not a plain token", c)
		}
	}
	if Class("anything_else").Valid() {
		t.Fatal("open class accepted")
	}
}

func TestRawErrorAndErrValueBecomeClassGroup(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	var called bool
	e := fmt.Errorf("call: %w", metaErr{ClassModelUnavailable.String(), 503, &called})
	called = false // fmt.Errorf with %w called Error() at construction
	l.ErrorContext(bg, "llm call failed", "err", e)
	l.ErrorContext(bg, "llm call failed", "err", Err(e))
	l.ErrorContext(bg, "llm call failed", "err", errors.New(sentBOM))
	called = false
	l.ErrorContext(bg, "llm call failed", "other", e)
	out := buf.String()
	assertClean(t, out)
	if called {
		t.Fatal("Error() was called")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var m map[string]map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &m)
	if m["err"]["class"] != "model_unavailable" || m["err"]["http_status"] != float64(503) {
		t.Fatalf("group wrong: %s", lines[0])
	}
	if !strings.Contains(lines[2], `"class":"unclassified"`) {
		t.Fatalf("unknown must be unclassified: %s", lines[2])
	}
	if strings.Contains(lines[3], "model_unavailable") {
		t.Fatalf("error under an unknown key must be dropped: %s", lines[3])
	}
}

func TestErrValueWorksWithPlainHandler(t *testing.T) {
	// Without the sanitiser the LogValuer still yields only the class group.
	var sb strings.Builder
	l := slog.New(slog.NewJSONHandler(&sb, nil))
	l.Error("x", "err", Err(errors.New(sentBOM)))
	assertClean(t, sb.String())
}
